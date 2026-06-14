/// ssanime-gui desktop shell — Tauri v2
///
/// Lifecycle:
///   1. setup() runs on the main thread: creates the HIDDEN "main" window, loading
///      the bundled placeholder page (desktop/src-tauri/ui-placeholder/index.html),
///      and builds the system tray (Open / Quit). A normal launch then shows the
///      window; a `--hidden` launch (the installer's autostart entry) leaves it in
///      the tray only.
///   2. A background std::thread spawns the Go sidecar with --no-open --headless.
///   3. The background thread polls 127.0.0.1:4773 (TCP) for up to 30s.
///   4. Once the daemon accepts connections, it calls window.navigate(daemon_url)
///      to redirect the window to the real UI.
///   5. Closing the window is intercepted (prevent_close + hide) so the daemon keeps
///      running in the background — only tray "Quit" (app.exit(0)) really exits.
///   6. On real exit (RunEvent::ExitRequested) the sidecar child is killed so no
///      orphan remains.
///
/// Orphan safety on Windows:
///   RunEvent::ExitRequested covers a graceful quit, but NOT a crash or a Task-Manager
///   force-kill of this process. To guarantee the sidecar never outlives us, on
///   Windows we put it in a Job Object flagged JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
///   The job's only handle is held by this process; when we die — by any means —
///   the OS closes that handle and kills everything in the job (the sidecar).
///
/// Why the window is created in setup() rather than a background thread:
///   On Windows, WebView2 window creation must happen on the main OS thread.
///   setup() is called on the main thread. Attempting to build a WebviewWindow from
///   a spawned thread (or async task) silently produces no window on Windows.
///
/// Why we use std::process::Command rather than tauri_plugin_shell::sidecar():
///   The shell plugin's sidecar() is primarily designed for JS frontend access control.
///   From Rust we directly launch the process; this avoids the plugin's path resolution
///   logic that can fail in debug/standalone builds.
use std::{
    net::TcpStream,
    process::{Child, Command},
    sync::{Arc, Mutex},
    time::{Duration, Instant},
};

use tauri::{
    menu::{MenuBuilder, MenuItem},
    tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent},
    AppHandle, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder, WindowEvent,
};

const DAEMON_ADDR: &str = "127.0.0.1:4773";
const DAEMON_URL: &str = "http://127.0.0.1:4773/";
const POLL_TIMEOUT: Duration = Duration::from_secs(30);
const POLL_INTERVAL: Duration = Duration::from_millis(200);

/// Global handle to the sidecar child so we can kill it on exit.
type SharedChild = Arc<Mutex<Option<Child>>>;

/// Windows: create a Job Object configured to kill all member processes when its
/// last handle closes, then assign `child` to it. The returned handle MUST be kept
/// alive for the whole process lifetime — dropping/closing it kills the sidecar
/// immediately. We intentionally leak it (store in a static) so it lives until we exit.
#[cfg(windows)]
fn confine_to_job(child: &Child) {
    use std::os::windows::io::AsRawHandle;
    use windows::Win32::Foundation::HANDLE;
    use windows::Win32::System::JobObjects::{
        AssignProcessToJobObject, CreateJobObjectW, SetInformationJobObject,
        JobObjectExtendedLimitInformation, JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
        JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    };

    unsafe {
        let job = match CreateJobObjectW(None, None) {
            Ok(j) => j,
            Err(e) => {
                eprintln!("[ssanime-desktop] CreateJobObject failed: {e}");
                return;
            }
        };

        let mut info = JOBOBJECT_EXTENDED_LIMIT_INFORMATION::default();
        info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
        let set_res: windows::core::Result<()> = SetInformationJobObject(
            job,
            JobObjectExtendedLimitInformation,
            &info as *const _ as *const std::ffi::c_void,
            std::mem::size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
        );
        if let Err(e) = set_res {
            eprintln!("[ssanime-desktop] SetInformationJobObject failed: {e}");
            return;
        }

        let child_handle = HANDLE(child.as_raw_handle() as _);
        let assign_res: windows::core::Result<()> = AssignProcessToJobObject(job, child_handle);
        if let Err(e) = assign_res {
            eprintln!("[ssanime-desktop] AssignProcessToJobObject failed: {e}");
            return;
        }

        // The job HANDLE is a Copy newtype with no Drop, and we never CloseHandle it,
        // so the OS keeps it open until this process exits — exactly the lifetime we
        // want. On exit (graceful or crash) the OS closes it → KILL_ON_JOB_CLOSE fires.
        let _ = job;
        eprintln!("[ssanime-desktop] sidecar confined to kill-on-close job object");
    }
}

/// Resolve the path to the Go sidecar binary.
///
/// In release bundles the binary is placed next to (or in `binaries/` relative to)
/// the main executable by the Tauri bundler. In debug builds we copy it to
/// `target/debug/binaries/ssanime-x86_64-pc-windows-msvc.exe` (done by the build
/// or the Taskfile). The name appended is the Rust target triple so the same logic
/// works on all platforms.
fn sidecar_path() -> std::path::PathBuf {
    let triple = std::env::consts::ARCH.to_owned()
        + "-"
        + {
            #[cfg(target_os = "windows")]
            { "pc-windows-msvc" }
            #[cfg(target_os = "macos")]
            { "apple-darwin" }
            #[cfg(target_os = "linux")]
            { "unknown-linux-gnu" }
        };

    let exe_dir = std::env::current_exe()
        .expect("cannot resolve current exe")
        .parent()
        .expect("exe has no parent dir")
        .to_path_buf();

    let ext = if cfg!(target_os = "windows") { ".exe" } else { "" };
    let triple_name = format!("ssanime-{triple}{ext}");
    // Tauri's bundler STRIPS the target-triple suffix when it copies an externalBin
    // next to the main exe, so in an installed/built app the sidecar is just `ssanime.exe`.
    let plain_name = format!("ssanime{ext}");

    // Try every layout we might encounter, in order of likelihood:
    //   1. bundled next to the exe, suffix stripped  → ssanime.exe          (installed / `tauri build`)
    //   2. next to the exe with the triple suffix    → ssanime-<triple>.exe (some dev runs)
    //   3. in a binaries/ subdir, either name        → dev / Taskfile copy layout
    let candidates = [
        exe_dir.join(&plain_name),
        exe_dir.join(&triple_name),
        exe_dir.join("binaries").join(&triple_name),
        exe_dir.join("binaries").join(&plain_name),
    ];
    for c in &candidates {
        if c.exists() {
            return c.clone();
        }
    }
    // Nothing found — return the most likely production path so the error message is useful.
    exe_dir.join(&plain_name)
}

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

/// Spawn the daemon sidecar and, once it is ready, navigate the already-visible
/// "main" window to the daemon URL. Runs entirely in a background thread so it
/// never blocks the main thread (and therefore never blocks setup()).
fn launch_daemon_and_navigate(app: AppHandle, child_handle: SharedChild) {
    std::thread::spawn(move || {
        let path = sidecar_path();
        eprintln!("[ssanime-desktop] spawning sidecar: {}", path.display());

        // --no-open: don't open a browser tab (Tauri window is the UI).
        // --headless: skip the systray (Tauri owns the process lifetime).
        // stdin(piped): give the daemon a pipe so it can detect OUR death (even a
        // crash/force-kill closes the pipe → the headless daemon sees EOF and exits,
        // so it never orphans). The ChildStdin stays open because we keep the Child.
        let spawn_result = Command::new(&path)
            .args(["--no-open", "--headless"])
            .stdin(std::process::Stdio::piped())
            .spawn();

        match spawn_result {
            Err(e) => {
                eprintln!(
                    "[ssanime-desktop] failed to spawn sidecar at {}: {e}",
                    path.display()
                );
                // Still navigate — the webview will show a connection error, which
                // is better than showing the loading spinner forever.
            }
            Ok(child) => {
                // Windows: bind the sidecar to a kill-on-close job BEFORE storing it,
                // so even an immediate crash of this process tears the sidecar down.
                #[cfg(windows)]
                confine_to_job(&child);
                *child_handle.lock().unwrap() = Some(child);
            }
        }

        // Wait for daemon to accept TCP connections.
        let ready = wait_for_daemon(POLL_TIMEOUT);
        if !ready {
            eprintln!(
                "[ssanime-desktop] daemon did not become ready within {}s — navigating anyway",
                POLL_TIMEOUT.as_secs()
            );
        }

        // Navigate the already-visible window to the daemon UI.
        // app.get_webview_window is safe to call from any thread.
        match app.get_webview_window("main") {
            None => {
                eprintln!("[ssanime-desktop] 'main' window not found; cannot navigate");
            }
            Some(win) => {
                let url: tauri::Url = DAEMON_URL.parse().expect("DAEMON_URL is a valid URL");
                if let Err(e) = win.navigate(url) {
                    eprintln!("[ssanime-desktop] navigate failed: {e}");
                }
            }
        }
    });
}

/// Bring the "main" window to the foreground: restore if minimized, show if
/// hidden, then focus. Used by the tray "Open" item, tray left-click, and the
/// single-instance handler.
fn reveal_main_window(app: &AppHandle) {
    if let Some(win) = app.get_webview_window("main") {
        let _ = win.unminimize();
        let _ = win.show();
        let _ = win.set_focus();
    }
}

/// True when this process was launched with `--hidden` (the autostart entry the
/// installer writes uses this so the app boots straight to the tray).
fn launched_hidden() -> bool {
    std::env::args().any(|a| a == "--hidden")
}

/// Kill the sidecar child if one is stored.
fn kill_sidecar(child_handle: &SharedChild) {
    if let Ok(mut guard) = child_handle.lock() {
        if let Some(mut child) = guard.take() {
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
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            // A second instance was launched — reveal our existing window.
            reveal_main_window(app);
        }))
        .setup(move |app| {
            // Create the window HERE on the main thread so Windows WebView2 can
            // initialise its COM/UI objects correctly. It is built HIDDEN; we
            // decide below whether to show it (normal launch) or leave it in the
            // tray (`--hidden`, used by the autostart entry). The background
            // thread calls navigate() once the daemon is ready.
            // Display name comes from tauri.conf.json `productName` — single source of truth.
            let app_name = app.package_info().name.clone();
            let loading_url = WebviewUrl::App("index.html".into());
            let main_window = WebviewWindowBuilder::new(app, "main", loading_url)
                .title(app_name.clone())
                .inner_size(1280.0, 800.0)
                .min_inner_size(800.0, 500.0)
                .resizable(true)
                .visible(false)
                .build()?;

            // System tray: Open + Quit.
            let open_item = MenuItem::with_id(app, "open", "Open", true, None::<&str>)?;
            let quit_item = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;
            let tray_menu = MenuBuilder::new(app)
                .item(&open_item)
                .separator()
                .item(&quit_item)
                .build()?;

            TrayIconBuilder::with_id("main-tray")
                .icon(app.default_window_icon().cloned().expect("bundle defines an icon"))
                .tooltip(app_name.clone())
                .menu(&tray_menu)
                .show_menu_on_left_click(false)
                .on_menu_event(|app, event| match event.id().as_ref() {
                    "open" => reveal_main_window(app),
                    "quit" => app.exit(0),
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| {
                    // Left-click the tray icon → reveal the window.
                    if let TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } = event
                    {
                        reveal_main_window(tray.app_handle());
                    }
                })
                .build(app)?;

            // Hide-to-tray on close: intercept the close request, keep the daemon
            // and process alive, just hide the window. MUST stay synchronous — an
            // async/block_on body here deadlocks prevent_close (Tauri #12334).
            let close_target = main_window.clone();
            main_window.on_window_event(move |event| {
                if let WindowEvent::CloseRequested { api, .. } = event {
                    api.prevent_close();
                    let _ = close_target.hide();
                }
            });

            // Normal launch shows the window; `--hidden` (autostart) leaves it in
            // the tray only.
            if !launched_hidden() {
                let _ = main_window.show();
                let _ = main_window.set_focus();
            }

            // Kick off daemon spawn + readiness poll in a background thread.
            let handle = app.handle().clone();
            launch_daemon_and_navigate(handle, child_for_setup);

            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(move |_app, event| {
            // Kill the sidecar only on a REAL exit (tray Quit → app.exit(0), or
            // OS shutdown), NOT on window-close (which we intercept and turn into
            // a hide — the daemon must keep running in the background). ExitRequested
            // fires once at teardown; CloseRequested fires on every hide.
            if let RunEvent::ExitRequested { .. } = event {
                kill_sidecar(&child_for_exit);
            }
        });
}
