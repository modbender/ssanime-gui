/// ssanime-gui desktop shell — Tauri v2
///
/// Lifecycle:
///   1. App starts up.
///   2. setup() spawns the Go sidecar (`binaries/ssanime`) with `--no-open`.
///   3. A background task polls `127.0.0.1:4773` (TCP) until the daemon accepts.
///   4. Once ready, it opens a `WebviewWindow` pointed at `http://127.0.0.1:4773/`.
///   5. On app exit (`RunEvent::Exit`), the sidecar child is killed so no orphan remains.
use std::{
    net::TcpStream,
    sync::{Arc, Mutex},
    time::{Duration, Instant},
};

use tauri::{AppHandle, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder};
use tauri_plugin_shell::{process::CommandChild, ShellExt};

const DAEMON_ADDR: &str = "127.0.0.1:4773";
const DAEMON_URL: &str = "http://127.0.0.1:4773/";
const POLL_TIMEOUT: Duration = Duration::from_secs(30);
const POLL_INTERVAL: Duration = Duration::from_millis(200);

/// Global handle to the sidecar child so we can kill it on exit.
type SharedChild = Arc<Mutex<Option<CommandChild>>>;

/// Poll `DAEMON_ADDR` with TCP connects until it accepts or timeout elapses.
/// Returns `true` if the daemon became ready within the timeout.
fn wait_for_daemon(timeout: Duration) -> bool {
    let deadline = Instant::now() + timeout;
    while Instant::now() < deadline {
        if TcpStream::connect(DAEMON_ADDR).is_ok() {
            return true;
        }
        std::thread::sleep(POLL_INTERVAL);
    }
    false
}

/// Spawn the daemon in a background thread, wait for readiness, then open the main window.
fn launch_daemon_and_window(app: AppHandle, child_handle: SharedChild) {
    std::thread::spawn(move || {
        // Spawn sidecar with --no-open so it doesn't open its own browser tab.
        let shell = app.shell();
        let result = shell
            .sidecar("binaries/ssanime")
            .and_then(|cmd| cmd.args(["--no-open"]).spawn());

        match result {
            Err(e) => {
                eprintln!("[ssanime-desktop] failed to spawn sidecar: {e}");
                return;
            }
            Ok((_rx, child)) => {
                // Store child so we can kill it on exit.
                *child_handle.lock().unwrap() = Some(child);
            }
        }

        // Wait for daemon to accept TCP connections.
        if !wait_for_daemon(POLL_TIMEOUT) {
            eprintln!(
                "[ssanime-desktop] daemon did not become ready within {}s",
                POLL_TIMEOUT.as_secs()
            );
            // Still attempt to open; the webview will show its own error if it can't connect.
        }

        // Open the main window on the Tauri runtime thread.
        let url = WebviewUrl::External(DAEMON_URL.parse().expect("static URL is valid"));
        let app_clone = app.clone();
        tauri::async_runtime::spawn(async move {
            let build_result = WebviewWindowBuilder::new(&app_clone, "main", url)
                .title("ssanime-gui")
                .inner_size(1280.0, 800.0)
                .min_inner_size(800.0, 500.0)
                .resizable(true)
                .visible(true)
                .build();

            if let Err(e) = build_result {
                eprintln!("[ssanime-desktop] failed to create window: {e}");
            }
        });
    });
}

/// Kill the sidecar child if one is stored.
fn kill_sidecar(child_handle: &SharedChild) {
    if let Ok(mut guard) = child_handle.lock() {
        if let Some(child) = guard.take() {
            if let Err(e) = child.kill() {
                eprintln!("[ssanime-desktop] failed to kill sidecar: {e}");
            }
        }
    }
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let child_handle: SharedChild = Arc::new(Mutex::new(None));
    let child_for_setup = child_handle.clone();
    let child_for_exit = child_handle.clone();

    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // A second instance was launched — focus our existing window.
            if let Some(win) = app.get_webview_window("main") {
                let _ = win.show();
                let _ = win.set_focus();
            }
        }))
        .setup(move |app| {
            let handle = app.handle().clone();
            launch_daemon_and_window(handle, child_for_setup);
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(move |_app, event| {
            if let RunEvent::Exit = event {
                kill_sidecar(&child_for_exit);
            }
        });
}
