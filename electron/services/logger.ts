import * as fs from 'fs';
import * as path from 'path';
import { app } from 'electron';

/**
 * Log levels in order of severity
 */
export enum LogLevel {
  DEBUG = 0,
  INFO = 1,
  WARN = 2,
  ERROR = 3,
}

interface LogOptions {
  /** The directory where log files will be stored */
  logDirectory?: string;
  /** Maximum size of a log file in bytes before rotation (default: 5MB) */
  maxFileSize?: number;
  /** Maximum number of log files to keep (default: 5) */
  maxFiles?: number;
  /** Minimum log level to output (default: DEBUG in development, INFO in production) */
  minLevel?: LogLevel;
  /** Whether to log to console in addition to file */
  console?: boolean;
}

/**
 * Logger service that writes logs to both console and files
 */
export class Logger {
  private category: string;
  private logFilePath: string;
  private options: Required<LogOptions>;
  private writeStream: fs.WriteStream | null = null;
  private currentFileSize: number = 0;

  /**
   * Create a logger instance
   * @param category Category name to prepend to log messages (e.g. 'Main', 'Encoder')
   * @param options Logging options
   */
  constructor(category: string, options: LogOptions = {}) {
    this.category = category;

    // Set default options
    const isDevelopment = process.env.NODE_ENV === 'development';

    this.options = {
      logDirectory: options.logDirectory || this.getDefaultLogDirectory(),
      maxFileSize: options.maxFileSize || 5 * 1024 * 1024, // 5MB
      maxFiles: options.maxFiles || 5,
      minLevel:
        options.minLevel ?? (isDevelopment ? LogLevel.DEBUG : LogLevel.INFO),
      console: options.console ?? true,
    };

    // Create log directory if it doesn't exist
    if (!fs.existsSync(this.options.logDirectory)) {
      fs.mkdirSync(this.options.logDirectory, { recursive: true });
    }

    // Set up log file path
    this.logFilePath = this.generateLogFilePath();

    // Initialize the log file
    this.initLogFile();
  }

  /**
   * Get the default log directory in the app's user data folder
   */
  private getDefaultLogDirectory(): string {
    let userDataPath;

    if (app) {
      // In Electron main process
      userDataPath = app.getPath('userData');
    } else {
      // Fallback if we're in renderer or testing
      userDataPath = path.join(
        process.env.APPDATA || process.env.HOME || '.',
        'SSAnime'
      );
    }

    return path.join(userDataPath, 'logs');
  }

  /**
   * Generate a log file path based on the current date
   */
  private generateLogFilePath(): string {
    const now = new Date();
    const dateStr = now.toISOString().split('T')[0]; // YYYY-MM-DD
    return path.join(this.options.logDirectory, `ssanime-${dateStr}.log`);
  }

  /**
   * Initialize or rotate the log file
   */
  private initLogFile(): void {
    try {
      // Check if file exists and its size
      if (fs.existsSync(this.logFilePath)) {
        const stats = fs.statSync(this.logFilePath);
        this.currentFileSize = stats.size;

        // Rotate if needed
        if (this.currentFileSize >= this.options.maxFileSize) {
          this.rotateLogFile();
        }
      } else {
        this.currentFileSize = 0;
      }

      // Close existing stream if any
      if (this.writeStream) {
        this.writeStream.end();
      }

      // Open a write stream in append mode
      this.writeStream = fs.createWriteStream(this.logFilePath, { flags: 'a' });

      // Write header if it's a new file
      if (this.currentFileSize === 0) {
        const header = `=== SSAnime GUI Log Started at ${new Date().toISOString()} ===\n`;
        this.writeStream.write(header);
        this.currentFileSize += header.length;
      }
    } catch (error) {
      console.error(`[Logger] Failed to initialize log file: ${error}`);
      // Disable file logging if we hit an error
      this.writeStream = null;
    }
  }

  /**
   * Rotate log files when they reach the maximum size
   */
  private rotateLogFile(): void {
    try {
      // Close current write stream if any
      if (this.writeStream) {
        this.writeStream.end();
        this.writeStream = null;
      }

      // Get existing log files
      const logDir = this.options.logDirectory;
      const logFiles = fs
        .readdirSync(logDir)
        .filter(file => file.startsWith('ssanime-') && file.endsWith('.log'))
        .sort((a, b) => {
          // Sort by creation time, newest first
          const timeA = fs.statSync(path.join(logDir, a)).mtime.getTime();
          const timeB = fs.statSync(path.join(logDir, b)).mtime.getTime();
          return timeB - timeA;
        });

      // Create a new log file with a timestamp
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
      const newPath = this.logFilePath.replace('.log', `-${timestamp}.log`);
      fs.renameSync(this.logFilePath, newPath);

      // Delete oldest files if we exceed maxFiles
      if (logFiles.length >= this.options.maxFiles) {
        // Keep the newest maxFiles-1 (because we're adding a new one)
        const filesToDelete = logFiles.slice(this.options.maxFiles - 1);
        for (const file of filesToDelete) {
          fs.unlinkSync(path.join(logDir, file));
        }
      }

      // Reset the current file size
      this.currentFileSize = 0;
    } catch (error) {
      console.error(`[Logger] Failed to rotate log files: ${error}`);
    }
  }

  /**
   * Format a log message with timestamp, category, and level
   */
  private formatLogMessage(
    level: string,
    message: string,
    args: any[]
  ): string {
    const timestamp = new Date().toISOString();
    let formattedMessage = `[${timestamp}] [${this.category}] [${level}] ${message}`;

    // Add formatted args if any
    if (args.length > 0) {
      try {
        formattedMessage +=
          ' ' +
          args
            .map(arg => {
              if (typeof arg === 'object') {
                return JSON.stringify(arg);
              }
              return String(arg);
            })
            .join(' ');
      } catch (error) {
        formattedMessage += ` (Error stringifying args: ${error})`;
      }
    }

    return formattedMessage;
  }

  /**
   * Write a log message to both console and file
   */
  private log(
    level: LogLevel,
    levelName: string,
    message: string,
    ...args: any[]
  ): void {
    // Skip if below minimum level
    if (level < this.options.minLevel) {
      return;
    }

    // Format the message
    const formattedMessage = this.formatLogMessage(levelName, message, args);

    // Log to console if enabled
    if (this.options.console) {
      switch (level) {
        case LogLevel.ERROR:
          console.error(formattedMessage);
          break;
        case LogLevel.WARN:
          console.warn(formattedMessage);
          break;
        case LogLevel.INFO:
          console.info(formattedMessage);
          break;
        case LogLevel.DEBUG:
        default:
          console.log(formattedMessage);
          break;
      }
    }

    // Log to file if stream is available
    if (this.writeStream) {
      try {
        // Add newline for file
        const messageWithNewline = formattedMessage + '\n';

        // Write to file
        this.writeStream.write(messageWithNewline);

        // Update current file size
        this.currentFileSize += messageWithNewline.length;

        // Check if we need to rotate
        if (this.currentFileSize >= this.options.maxFileSize) {
          this.rotateLogFile();
          this.initLogFile();
        }
      } catch (error) {
        console.error(`[Logger] Failed to write to log file: ${error}`);
      }
    }
  }

  /**
   * Log a debug message
   */
  debug(message: string, ...args: any[]): void {
    this.log(LogLevel.DEBUG, 'DEBUG', message, ...args);
  }

  /**
   * Log an info message
   */
  info(message: string, ...args: any[]): void {
    this.log(LogLevel.INFO, 'INFO', message, ...args);
  }

  /**
   * Log a warning message
   */
  warn(message: string, ...args: any[]): void {
    this.log(LogLevel.WARN, 'WARN', message, ...args);
  }

  /**
   * Log an error message
   */
  error(message: string, ...args: any[]): void {
    this.log(LogLevel.ERROR, 'ERROR', message, ...args);
  }

  /**
   * Flush and close the log file
   * Should be called when the application exits
   */
  closeLog(): void {
    if (this.writeStream) {
      const footer = `=== SSAnime GUI Log Closed at ${new Date().toISOString()} ===\n`;
      this.writeStream.write(footer);
      this.writeStream.end();
      this.writeStream = null;
    }
  }

  /**
   * Get the path to the current log file
   * Useful for showing users where to find logs
   */
  getLogFilePath(): string {
    return this.logFilePath;
  }
}

// Export a factory function to create loggers
export function createLogger(category: string, options?: LogOptions): Logger {
  return new Logger(category, options);
}

// Export a default logger for quick access
export const defaultLogger = createLogger('App');
