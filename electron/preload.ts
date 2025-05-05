import { contextBridge, ipcRenderer } from 'electron';

// Define API exposed to renderer process
contextBridge.exposeInMainWorld('ipcRenderer', {
  invoke: (channel: string, ...args: any[]) => {
    const validChannels = [
      'app-start-time',
      'get-encoding-profiles',
      'save-encoding-profiles',
      'is-default-profile',
      'select-files',
      'select-output-directory',
      'start-encoding',
      'stop-encoding',
      'get-encoding-progress',
      'get-recent-output-paths',
      'get-most-recent-output-path',
      'clear-path-history',
      'get-log-file-path',
    ];

    if (validChannels.includes(channel)) {
      return ipcRenderer.invoke(channel, ...args);
    }

    throw new Error(`Unauthorized IPC channel: ${channel}`);
  },

  on: (channel: string, listener: (...args: any[]) => void) => {
    const validChannels = ['encoding-completed', 'encoding-error'];

    if (validChannels.includes(channel)) {
      ipcRenderer.on(channel, (event, ...args) => listener(...args));
    }
  },

  off: (channel: string, listener?: (...args: any[]) => void) => {
    const validChannels = ['encoding-completed', 'encoding-error'];

    if (validChannels.includes(channel)) {
      if (listener) {
        ipcRenderer.off(channel, listener);
      } else {
        ipcRenderer.removeAllListeners(channel);
      }
    }
  },
});
