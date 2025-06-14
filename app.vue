<template>
  <div class="desktop-app" :class="{ dark: isDark }">
    <!-- Native-style title bar -->
    <div class="titlebar">
      <div class="titlebar-drag-region">
        <div class="titlebar-left">
          <div class="app-icon">
            <img src="/logo.svg" alt="SSAnime GUI" />
          </div>
          <span class="app-title">SSAnime GUI</span>
        </div>
        <div class="titlebar-right">
          <button
            v-tooltip.bottom="isDark ? 'Light Theme' : 'Dark Theme'"
            class="titlebar-button"
            @click="toggleTheme"
          >
            <i :class="isDark ? 'pi pi-sun' : 'pi pi-moon'" />
          </button>
          <button
            v-tooltip.bottom="'Minimize'"
            class="titlebar-button minimize-btn"
            @click="minimizeWindow"
          >
            <i class="pi pi-minus" />
          </button>
          <button
            v-tooltip.bottom="'Maximize'"
            class="titlebar-button maximize-btn"
            @click="maximizeWindow"
          >
            <i class="pi pi-window-maximize" />
          </button>
          <button
            v-tooltip.bottom="'Close'"
            class="titlebar-button close-btn"
            @click="closeWindow"
          >
            <i class="pi pi-times" />
          </button>
        </div>
      </div>
    </div>

    <!-- Native desktop layout -->
    <div class="desktop-layout">
      <!-- Sidebar navigation -->
      <div class="sidebar">
        <nav class="sidebar-nav">
          <div class="nav-section">
            <h3 class="nav-heading">Encoding</h3>
            <button
              v-tooltip.right="'Video Encoder'"
              class="nav-item"
              :class="{ active: currentView === 'encoder' }"
              @click="setActiveView('encoder')"
            >
              <i class="pi pi-play-circle" />
              <span>Encoder</span>
            </button>
            <button
              v-tooltip.right="'Encoding Profiles'"
              class="nav-item"
              :class="{ active: currentView === 'profiles' }"
              @click="setActiveView('profiles')"
            >
              <i class="pi pi-cog" />
              <span>Profiles</span>
            </button>
            <button
              v-tooltip.right="'Encoding Queue'"
              class="nav-item"
              :class="{ active: currentView === 'queue' }"
              @click="setActiveView('queue')"
            >
              <i class="pi pi-list" />
              <span>Queue</span>
            </button>
          </div>
          <div class="nav-section">
            <h3 class="nav-heading">Tools</h3>
            <button
              v-tooltip.right="'System Logs'"
              class="nav-item"
              :class="{ active: currentView === 'logs' }"
              @click="setActiveView('logs')"
            >
              <i class="pi pi-file-edit" />
              <span>Logs</span>
            </button>
            <button
              v-tooltip.right="'Application Settings'"
              class="nav-item"
              :class="{ active: currentView === 'settings' }"
              @click="setActiveView('settings')"
            >
              <i class="pi pi-sliders-h" />
              <span>Settings</span>
            </button>
          </div>
        </nav>
      </div>

      <!-- Main content area -->
      <div class="main-area">
        <div class="content-header">
          <div class="content-title">
            <h2>{{ getViewTitle() }}</h2>
            <p class="content-subtitle">{{ getViewSubtitle() }}</p>
          </div>
          <div class="content-actions">
            <!-- View-specific actions will go here -->
          </div>
        </div>
        <div class="content-body">
          <EncodingMain
            v-if="currentView === 'encoder'"
            @navigate-to-profiles="setActiveView('profiles')"
          />
          <EncodingProfiles v-if="currentView === 'profiles'" />
          <EncodingQueue v-if="currentView === 'queue'" />
          <SystemLogs v-if="currentView === 'logs'" />
          <SettingsApp v-if="currentView === 'settings'" />
        </div>
      </div>
    </div>

    <!-- Status bar -->
    <div class="statusbar">
      <div class="statusbar-left">
        <span class="status-item">
          <i
            class="pi pi-circle-fill"
            :class="{ 'text-success': !isEncoding, 'text-warning': isEncoding }"
          />
          {{ isEncoding ? 'Encoding in progress' : 'Ready' }}
        </span>
      </div>
      <div class="statusbar-right">
        <span v-if="selectedFiles.length > 0" class="status-item">
          {{ selectedFiles.length }} files selected
        </span>
        <span v-if="activeProfile" class="status-item">
          Profile: {{ activeProfile }}
        </span>
      </div>
    </div>

    <Toast position="bottom-right" />
  </div>
</template>

<script setup>
// No manual imports needed - Nuxt 3 auto-imports Vue and composables

// Theme state
const isDark = ref(true);
const toast = useToast();

// Navigation state
const currentView = ref('encoder');
const isEncoding = ref(false);
const selectedFiles = ref([]);
const activeProfile = ref('Default Profile');

// View definitions
const views = {
  encoder: {
    title: 'Video Encoder',
    subtitle: 'Encode your video files with custom settings',
    component: 'EncoderMain',
  },
  profiles: {
    title: 'Encoding Profiles',
    subtitle: 'Manage your encoding presets and configurations',
    component: 'EncodingProfiles',
  },
  queue: {
    title: 'Encoding Queue',
    subtitle: 'Monitor and manage your encoding tasks',
    component: 'EncodingQueue',
  },
  logs: {
    title: 'System Logs',
    subtitle: 'View encoding logs and system information',
    component: 'SystemLogs',
  },
  settings: {
    title: 'Settings',
    subtitle: 'Configure application preferences',
    component: 'AppSettings',
  },
};

// Computed properties
const getViewTitle = () => views[currentView.value]?.title || 'Unknown View';
const getViewSubtitle = () => views[currentView.value]?.subtitle || '';

// Theme functions
const toggleTheme = () => {
  isDark.value = !isDark.value;
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light');

  // Update document class for global theme changes
  document.documentElement.classList.toggle('dark', isDark.value);

  toast.add({
    severity: 'info',
    summary: 'Theme Changed',
    detail: `Switched to ${isDark.value ? 'Dark' : 'Light'} theme`,
    life: 2000,
  });
};

// Window controls (for Electron)
const minimizeWindow = () => {
  if (window.ipcRenderer) {
    window.ipcRenderer.invoke('window-minimize');
  }
};

const maximizeWindow = () => {
  if (window.ipcRenderer) {
    window.ipcRenderer.invoke('window-maximize');
  }
};

const closeWindow = () => {
  if (window.ipcRenderer) {
    window.ipcRenderer.invoke('window-close');
  }
};

// Navigation functions
const setActiveView = view => {
  currentView.value = view;
  toast.add({
    severity: 'info',
    summary: 'View Changed',
    detail: `Switched to ${views[view]?.title}`,
    life: 1500,
  });
};

// Initialize app
onMounted(() => {
  // Load saved theme
  const savedTheme = localStorage.getItem('theme');
  if (savedTheme) {
    isDark.value = savedTheme === 'dark';
  }

  // Apply theme to document
  document.documentElement.classList.toggle('dark', isDark.value);

  // Simulate some initial state (replace with real data)
  setTimeout(() => {
    selectedFiles.value = ['video1.mp4', 'video2.mkv'];
  }, 1000);
});
</script>

<style scoped>
/* Native Desktop Application Styles */
.desktop-app {
  display: flex;
  flex-direction: column;
  height: 100vh;
  width: 100vw;
  background: var(--bg-primary);
  color: var(--text-primary);
  font-family:
    -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', sans-serif;
  overflow: hidden;
  user-select: none;
}

/* Native Title Bar */
.titlebar {
  height: 30px;
  background: var(--bg-secondary);
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
  -webkit-app-region: drag;
}

.titlebar-drag-region {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 100%;
  padding: 0 8px;
}

.titlebar-left {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
}

.app-icon {
  width: 16px;
  height: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.app-icon img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.app-title {
  font-size: 12px;
  font-weight: 400;
}

.titlebar-right {
  display: flex;
  align-items: center;
  gap: 2px;
  -webkit-app-region: no-drag;
}

.titlebar-button {
  width: 46px;
  height: 30px;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.15s ease;
  font-size: 10px;
}

.titlebar-button:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.titlebar-button.close-btn:hover {
  background: #ff5f57;
  color: white;
}

.titlebar-button.minimize-btn:hover {
  background: #ffbd2e;
  color: white;
}

.titlebar-button.maximize-btn:hover {
  background: #28ca42;
  color: white;
}

/* Desktop Layout */
.desktop-layout {
  display: flex;
  flex: 1;
  overflow: hidden;
}

/* Sidebar Navigation */
.sidebar {
  width: 240px;
  background: var(--bg-secondary);
  border-right: 1px solid var(--border-color);
  flex-shrink: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

.sidebar-nav {
  padding: 16px 8px;
}

.nav-section {
  margin-bottom: 24px;
}

.nav-section:last-child {
  margin-bottom: 0;
}

.nav-heading {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--text-muted);
  margin: 0 0 8px 12px;
  padding: 0;
}

.nav-item {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 12px;
  margin: 2px 0;
  border: none;
  background: transparent;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 400;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  text-align: left;
}

.nav-item:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.nav-item.active {
  background: var(--primary-color);
  color: white;
}

.nav-item.active:hover {
  background: var(--primary-dark);
}

.nav-item i {
  font-size: 14px;
  width: 16px;
  text-align: center;
  flex-shrink: 0;
}

.nav-item span {
  flex: 1;
}

/* Main Content Area */
.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  background: var(--bg-primary);
  overflow: hidden;
}

.content-header {
  padding: 20px 24px;
  background: var(--bg-primary);
  border-bottom: 1px solid var(--border-color);
  flex-shrink: 0;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.content-title h2 {
  font-size: 20px;
  font-weight: 600;
  color: var(--text-primary);
  margin: 0 0 4px 0;
}

.content-subtitle {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0;
  font-weight: 400;
}

.content-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.content-body {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 0;
  background: var(--bg-primary);
}

/* Status Bar */
.statusbar {
  height: 22px;
  background: var(--bg-secondary);
  border-top: 1px solid var(--border-color);
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 12px;
  font-size: 11px;
  color: var(--text-secondary);
  flex-shrink: 0;
}

.statusbar-left,
.statusbar-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
}

.status-item i {
  font-size: 8px;
}

.text-success {
  color: var(--success-color);
}

.text-warning {
  color: var(--warning-color);
}

/* Dark Theme Specific Adjustments */
.dark .titlebar {
  background: #1e1e1e;
  border-bottom-color: #333333;
}

.dark .sidebar {
  background: #252526;
  border-right-color: #333333;
}

.dark .statusbar {
  background: #1e1e1e;
  border-top-color: #333333;
}

.dark .content-header {
  border-bottom-color: #333333;
}

/* Scrollbar Styling for Native Look */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

::-webkit-scrollbar-track {
  background: var(--bg-secondary);
}

::-webkit-scrollbar-thumb {
  background: var(--border-color);
  border-radius: 4px;
}

::-webkit-scrollbar-thumb:hover {
  background: var(--border-hover);
}

/* Custom scrollbar for dark theme */
.dark ::-webkit-scrollbar-track {
  background: #2d2d30;
}

.dark ::-webkit-scrollbar-thumb {
  background: #464647;
}

.dark ::-webkit-scrollbar-thumb:hover {
  background: #5a5a5c;
}

/* Animations */
@keyframes fadeIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}

.desktop-app {
  animation: fadeIn 0.3s ease-out;
}

/* Responsive adjustments */
@media (max-width: 768px) {
  .sidebar {
    width: 200px;
  }

  .content-header {
    padding: 16px 20px;
  }

  .content-title h2 {
    font-size: 18px;
  }
}

@media (max-width: 640px) {
  .sidebar {
    width: 180px;
  }

  .nav-item span {
    display: none;
  }

  .sidebar {
    width: 56px;
  }
}
</style>
