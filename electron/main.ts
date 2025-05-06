import { app, BrowserWindow, ipcMain, dialog } from 'electron';
import path from 'node:path';
import fs from 'node:fs';
import { Encoder } from './services/encoder';
import { ProfileManager } from './services/profiles';
import { PathHistoryService } from './services/path-history';
import { createLogger } from './services/logger';

// Initialize logger
const log = createLogger('Main');

// The built directory structure
//
// ├─┬ dist-electron
// │ ├─┬ main
// │ │ └── index.js
// │ ├─┬ preload
// │ │ └── index.js
// │ ├─┬ renderer
// │ │ └── index.html
process.env.APP_ROOT = path.join(__dirname, '..');

export const MAIN_DIST = path.join(process.env.APP_ROOT, 'dist-electron');
export const RENDERER_DIST = path.join(process.env.APP_ROOT, '.output/public');

process.env.VITE_PUBLIC = process.env.VITE_DEV_SERVER_URL
  ? path.join(process.env.APP_ROOT, 'public')
  : RENDERER_DIST;

let win: BrowserWindow | null;

// Services
let encoder: Encoder | null = null;
let profileManager: ProfileManager | null = null;
let pathHistory: PathHistoryService | null = null;

// Add manualStop property to the encoder class to track
const manualStopFlag = { value: false };

// Initialize services
function initServices() {
  log.info('Initializing services');
  // Create encoder and profile manager instances
  try {
    encoder = new Encoder();
    log.info('Encoder service initialized');
  } catch (error) {
    log.error('Failed to initialize encoder service:', error);
  }

  try {
    profileManager = new ProfileManager();
    log.info('Profile manager initialized');
  } catch (error) {
    log.error('Failed to initialize profile manager:', error);
  }

  try {
    pathHistory = new PathHistoryService();
    log.info('Path history service initialized');
  } catch (error) {
    log.error('Failed to initialize path history service:', error);
  }

  // Set encoder profiles from profile manager
  if (encoder && profileManager) {
    try {
      const profiles = profileManager.getProfiles();
      encoder.setProfiles(profiles);
      log.info('Profiles loaded into encoder service:', Object.keys(profiles));
    } catch (error) {
      log.error('Failed to set encoder profiles:', error);
    }
  } else {
    log.warn(
      'Could not set encoder profiles - services not initialized properly'
    );
  }
}

function createWindow() {
  log.info('Creating main window');
  win = new BrowserWindow({
    width: 1200,
    height: 800,
    webPreferences: {
      preload: path.join(MAIN_DIST, 'preload.js'),
      webSecurity: true,
      allowRunningInsecureContent: false,
      contextIsolation: true,
    },
    // Remove window rounding by using square corners
    frame: true,
    roundedCorners: false,
    // Remove application menu bar
    autoHideMenuBar: true,
  });

  // Remove the application menu completely
  win.setMenu(null);
  
  // Add CSP headers to allow SVG content and icons
  win.webContents.session.webRequest.onHeadersReceived((details, callback) => {
    callback({
      responseHeaders: {
        ...details.responseHeaders,
        'Content-Security-Policy': [
          "default-src 'self'; img-src 'self' data: https:; style-src 'self' 'unsafe-inline'; font-src 'self' data:; script-src 'self'; connect-src 'self'; object-src 'none'"
        ]
      }
    });
  });

  if (process.env.VITE_DEV_SERVER_URL) {
    log.info(
      'Running in development mode, loading URL:',
      process.env.VITE_DEV_SERVER_URL
    );
    win.loadURL(process.env.VITE_DEV_SERVER_URL);
    win.webContents.openDevTools();
  } else {
    const indexPath = path.join(process.env.VITE_PUBLIC!, 'index.html');
    log.info('Running in production mode, loading file:', indexPath);
    win.loadFile(indexPath);
  }

  win.webContents.on('did-finish-load', () => {
    log.info('Main window loaded successfully');
  });

  win.webContents.on('did-fail-load', (event, errorCode, errorDescription) => {
    log.error('Window failed to load:', { errorCode, errorDescription });
  });
}

function initIpc() {
  log.info('Initializing IPC handlers');

  // Basic app info
  ipcMain.handle('app-start-time', () => {
    log.debug('IPC: app-start-time requested');
    return new Date().toLocaleString();
  });

  // Get log file path
  ipcMain.handle('get-log-file-path', () => {
    log.debug('IPC: get-log-file-path requested');
    // Get the current log file path
    return log.getLogFilePath();
  });

  // Profile management
  ipcMain.handle('get-encoding-profiles', () => {
    log.debug('IPC: get-encoding-profiles requested');
    if (profileManager) {
      const profiles = profileManager.getProfiles();
      log.debug('Returning profiles:', Object.keys(profiles));
      return profiles;
    }
    log.error('Profile manager not available');
    return {};
  });

  ipcMain.handle('save-encoding-profiles', (_, profiles) => {
    log.debug('IPC: save-encoding-profiles requested', {
      profileNames: Object.keys(profiles),
    });
    if (profileManager) {
      try {
        profileManager.saveProfiles(profiles);
        log.info('Profiles saved successfully');

        // Update encoder with new profiles
        if (encoder) {
          encoder.setProfiles(profileManager.getProfiles());
          log.debug('Encoder updated with new profiles');
        }

        return { success: true };
      } catch (error) {
        log.error('Failed to save profiles:', error);
        return { success: false, error: String(error) };
      }
    }
    log.error('Profile manager not available');
    return { success: false, error: 'Profile manager not available' };
  });

  ipcMain.handle('is-default-profile', (_, profileName) => {
    log.debug('IPC: is-default-profile requested', { profileName });
    return profileManager
      ? profileManager.isDefaultProfile(profileName)
      : false;
  });

  // Path history management IPC handlers
  ipcMain.handle('get-recent-output-paths', () => {
    log.debug('IPC: get-recent-output-paths requested');
    if (!pathHistory) {
      log.error('Path history service not available');
      return [];
    }
    return pathHistory.getOutputPaths();
  });

  ipcMain.handle('get-most-recent-output-path', () => {
    log.debug('IPC: get-most-recent-output-path requested');
    if (!pathHistory) {
      log.error('Path history service not available');
      return null;
    }
    return pathHistory.getMostRecentOutputPath();
  });

  ipcMain.handle('clear-path-history', () => {
    log.debug('IPC: clear-path-history requested');
    if (!pathHistory) {
      log.error('Path history service not available');
      return { success: false };
    }

    pathHistory.clearHistory();
    return { success: true };
  });

  // File/directory selection
  ipcMain.handle('select-files', async () => {
    log.debug('IPC: select-files dialog requested');
    if (!win) {
      log.error('Cannot show dialog - window not available');
      return { canceled: true };
    }

    try {
      // Use most recent input path as the default path if available
      const defaultPath = pathHistory?.getMostRecentInputPath() || undefined;

      const result = await dialog.showOpenDialog(win, {
        defaultPath,
        properties: ['openFile', 'multiSelections'],
        filters: [
          {
            name: 'Video Files',
            extensions: ['mp4', 'mkv', 'avi', 'mov', 'wmv'],
          },
        ],
      });

      log.debug('File selection dialog result:', {
        canceled: result.canceled,
        fileCount: result.filePaths.length,
        files: result.filePaths.map((p) => path.basename(p)),
      });

      // If files were selected, store the parent directory path
      if (!result.canceled && result.filePaths.length > 0) {
        const parentDir = path.dirname(result.filePaths[0]);
        pathHistory?.addInputPath(parentDir);
        log.debug('Added input path to history:', parentDir);
      }

      return result;
    } catch (error) {
      log.error('Error during file selection dialog:', error);
      return { canceled: true, error: String(error) };
    }
  });

  ipcMain.handle('select-output-directory', async () => {
    log.debug('IPC: select-output-directory dialog requested');
    if (!win) {
      log.error('Cannot show dialog - window not available');
      return { canceled: true };
    }

    try {
      // Use most recent output path as the default path if available
      const defaultPath = pathHistory?.getMostRecentOutputPath() || undefined;

      const result = await dialog.showOpenDialog(win, {
        defaultPath,
        properties: ['openDirectory'],
      });

      log.debug('Directory selection dialog result:', {
        canceled: result.canceled,
        directory: result.filePaths[0],
      });

      // If a directory was selected, add it to the history
      if (!result.canceled && result.filePaths.length > 0) {
        pathHistory?.addOutputPath(result.filePaths[0]);
        log.debug('Added output path to history:', result.filePaths[0]);
      }

      return result;
    } catch (error) {
      log.error('Error during directory selection dialog:', error);
      return { canceled: true, error: String(error) };
    }
  });

  // Encoding operations
  ipcMain.handle('start-encoding', async (_, options) => {
    log.info('IPC: start-encoding requested', {
      fileCount: options?.files?.length || 0,
      outputDir: options?.outputDirectory,
      profileName: options?.profileName,
    });

    if (!encoder || !profileManager) {
      const error = 'Encoder or profile manager not available';
      log.error(error);
      return { success: false, error };
    }

    const { files, outputDirectory, profileName } = options;

    if (!files || !files.length) {
      const error = 'No input files specified';
      log.error(error);
      return { success: false, error };
    }

    if (!outputDirectory) {
      const error = 'No output directory specified';
      log.error(error);
      return { success: false, error };
    }

    if (!profileName) {
      const error = 'No encoding profile specified';
      log.error(error);
      return { success: false, error };
    }

    try {
      // Make sure output directory exists
      if (!fs.existsSync(outputDirectory)) {
        log.info(`Creating output directory: ${outputDirectory}`);
        fs.mkdirSync(outputDirectory, { recursive: true });
      }

      // Add the output directory to history
      if (pathHistory) {
        pathHistory.addOutputPath(outputDirectory);
        log.debug(
          'Added output path to history from encoding start:',
          outputDirectory
        );
      }

      // Start encoding in background and don't wait for completion
      log.info('Starting encoding process in background');
      encoder
        .encodeFiles(files, outputDirectory, profileName)
        .then(() => {
          log.info('Encoding process completed successfully');
          if (win) {
            win.webContents.send('encoding-completed');
            log.debug('Sent encoding-completed event to renderer');
          }
        })
        .catch((error) => {
          log.error('Encoding process failed:', error);
          if (win) {
            win.webContents.send('encoding-error', error.message);
            log.debug('Sent encoding-error event to renderer');
          }
        });

      return { success: true };
    } catch (error: any) {
      log.error('Failed to start encoding process:', error);
      return { success: false, error: error.message };
    }
  });

  ipcMain.handle('stop-encoding', () => {
    log.info('IPC: stop-encoding requested');
    if (encoder) {
      // Set manual stop flag
      manualStopFlag.value = true;
      encoder.stop();
      log.info('Encoding process stopped');

      // Notify renderer that encoding was manually stopped
      if (win) {
        win.webContents.send('encoding-stopped');
      }

      return { success: true };
    }
    log.error('Encoder not available');
    return { success: false, error: 'Encoder not available' };
  });

  ipcMain.handle('get-encoding-progress', () => {
    // This is called frequently, so we use debug level
    log.debug('IPC: get-encoding-progress requested');
    if (encoder) {
      const progress = encoder.getProgress();
      return progress;
    }
    log.warn('Encoder not available for progress check');
    return {
      percent: 0,
      currentFile: '',
      speed: 'N/A',
      eta: 'N/A',
      completed: true,
    };
  });

  // Handle parallel multi-resolution encoding request
  ipcMain.handle(
    'start-parallel-encoding',
    async (event, { file, outputDirectory, profileName }) => {
      try {
        log.info('Parallel encoding request received', {
          file,
          outputDirectory,
          profileName,
        });

        if (!profileManager) {
          log.error('Profile manager not initialized');
          return { success: false, error: 'Profile manager not initialized' };
        }

        if (!encoder) {
          log.error('Encoder not initialized');
          return { success: false, error: 'Encoder not initialized' };
        }

        // Initialize encoder with profiles
        encoder.setProfiles(profileManager.getProfiles());

        // Save the output directory to recent paths history
        if (!pathHistory) {
          log.warn('Path history service not initialized');
        } else {
          pathHistory.addOutputPath(outputDirectory);
        }

        // Start encoding - using standard encoding instead of parallel
        encoder
          .encode(file, outputDirectory, profileName)
          .then(() => {
            log.info('Encoding completed successfully');
            win?.webContents.send('encoding-completed');
          })
          .catch((error: Error) => {
            const errorMessage =
              error instanceof Error ? error.message : String(error);
            log.error('Error during encoding:', error);
            win?.webContents.send('encoding-error', errorMessage);
          });

        return { success: true };
      } catch (error: unknown) {
        const errorMessage =
          error instanceof Error ? error.message : String(error);
        log.error('Failed to start encoding:', error);
        return { success: false, error: errorMessage };
      }
    }
  );

  log.info('IPC handlers initialized');
}

app.on('window-all-closed', () => {
  log.info('All windows closed');
  if (process.platform !== 'darwin') {
    app.quit();
    win = null;
  }
});

app.on('activate', () => {
  log.info('App activated');
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  }
});

app.on('quit', () => {
  // Close the log file properly
  log.info('App quitting, closing log file');
});

app.whenReady().then(() => {
  log.info('App is ready, initializing application');
  initServices();
  initIpc();
  createWindow();
  log.info('Application startup complete');
});

// Handle uncaught exceptions
process.on('uncaughtException', (error) => {
  log.error('Uncaught exception in main process:', error);
});

process.on('unhandledRejection', (reason, promise) => {
  log.error('Unhandled rejection in main process:', { reason, promise });
});
