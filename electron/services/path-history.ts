import Store from 'electron-store';
import * as path from 'path';

// Define the structure of a path entry
interface PathEntry {
  path: string;
  lastUsed: number; // timestamp
  label?: string; // optional user-friendly name
}

// Define the store schema
interface PathHistoryStore {
  outputPaths: PathEntry[];
  inputPaths: PathEntry[];
  maxEntries: number;
}

export class PathHistoryService {
  private store: Store<PathHistoryStore>;

  constructor() {
    // Initialize the store with default values
    this.store = new Store<PathHistoryStore>({
      name: 'path-history', // Name of the config file
      defaults: {
        outputPaths: [],
        inputPaths: [],
        maxEntries: 10, // Maximum number of paths to remember
      },
    });
  }

  /**
   * Add a new output path to history
   * @param outputPath The path to add
   */
  addOutputPath(outputPath: string): void {
    if (!outputPath) return;

    // Normalize the path to handle different slash styles
    const normalizedPath = path.normalize(outputPath);

    // Get existing paths
    let paths = this.getOutputPaths();

    // Check if path already exists
    const existingIndex = paths.findIndex(p => p.path === normalizedPath);

    if (existingIndex >= 0) {
      // Path exists, update the timestamp
      paths[existingIndex].lastUsed = Date.now();
    } else {
      // Add new path
      paths.unshift({
        path: normalizedPath,
        lastUsed: Date.now(),
        label: path.basename(normalizedPath),
      });

      // Limit the number of entries
      const maxEntries = this.store.get('maxEntries');
      if (paths.length > maxEntries) {
        paths = paths.slice(0, maxEntries);
      }
    }

    // Sort by most recently used
    paths.sort((a, b) => b.lastUsed - a.lastUsed);

    // Save back to store
    this.store.set('outputPaths', paths);
  }

  /**
   * Add a new input path to history
   * @param inputPath The path to add
   */
  addInputPath(inputPath: string): void {
    if (!inputPath) return;

    // Normalize the path to handle different slash styles
    const normalizedPath = path.normalize(inputPath);

    // Get existing paths
    let paths = this.getInputPaths();

    // Check if path already exists
    const existingIndex = paths.findIndex(p => p.path === normalizedPath);

    if (existingIndex >= 0) {
      // Path exists, update the timestamp
      paths[existingIndex].lastUsed = Date.now();
    } else {
      // Add new path
      paths.unshift({
        path: normalizedPath,
        lastUsed: Date.now(),
        label: path.basename(normalizedPath),
      });

      // Limit the number of entries
      const maxEntries = this.store.get('maxEntries');
      if (paths.length > maxEntries) {
        paths = paths.slice(0, maxEntries);
      }
    }

    // Sort by most recently used
    paths.sort((a, b) => b.lastUsed - a.lastUsed);

    // Save back to store
    this.store.set('inputPaths', paths);
  }

  /**
   * Get all output paths sorted by recent usage
   */
  getOutputPaths(): PathEntry[] {
    return this.store.get('outputPaths');
  }

  /**
   * Get all input paths sorted by recent usage
   */
  getInputPaths(): PathEntry[] {
    return this.store.get('inputPaths');
  }

  /**
   * Get the most recently used output path
   */
  getMostRecentOutputPath(): string | null {
    const paths = this.getOutputPaths();
    return paths.length > 0 ? paths[0].path : null;
  }

  /**
   * Get the most recently used input directory
   */
  getMostRecentInputPath(): string | null {
    const paths = this.getInputPaths();
    return paths.length > 0 ? paths[0].path : null;
  }

  /**
   * Clear all saved paths
   */
  clearHistory(): void {
    this.store.set('outputPaths', []);
    this.store.set('inputPaths', []);
  }
}
