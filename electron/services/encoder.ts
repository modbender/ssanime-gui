import { spawn, ChildProcess } from 'child_process';
import * as path from 'path';
import * as fs from 'fs';
// Fix ffmpeg-static import to get the correct path to the binary
import ffmpegStatic from 'ffmpeg-static';
import { createLogger } from './logger';

// Initialize logger for encoder service
const log = createLogger('Encoder');

// Improved function to find FFmpeg binary
function findFFmpegPath(): string {
  // First try using ffmpeg-static
  let ffmpegPath = ffmpegStatic as unknown as string;
  log.debug(`FFmpeg path from ffmpeg-static: ${ffmpegPath}`);

  // Check if it exists
  if (ffmpegPath && fs.existsSync(ffmpegPath)) {
    log.info(`Found FFmpeg using ffmpeg-static at: ${ffmpegPath}`);
    return ffmpegPath;
  }

  // Second approach: Check common paths
  const possiblePaths = [
    // Common paths on Windows
    'ffmpeg.exe',
    path.join(process.cwd(), 'ffmpeg.exe'),
    path.join(process.cwd(), 'bin', 'ffmpeg.exe'),
    path.join(process.cwd(), 'resources', 'ffmpeg.exe'),
    // Path in node_modules
    path.join(process.cwd(), 'node_modules', 'ffmpeg-static', 'ffmpeg.exe'),
    path.join(
      process.cwd(),
      'node_modules',
      'ffmpeg-static',
      'ffmpeg-win32-x64.exe'
    ),
  ];

  for (const possiblePath of possiblePaths) {
    log.debug(`Checking for FFmpeg at: ${possiblePath}`);
    if (fs.existsSync(possiblePath)) {
      log.info(`Found FFmpeg at: ${possiblePath}`);
      return possiblePath;
    }
  }

  // As a last resort, hope it's in PATH
  log.warn('Could not find FFmpeg binary, will try to use from system PATH');
  return 'ffmpeg';
}

// Get the correct ffmpeg path
const ffmpegPath = findFFmpegPath();

export interface EncodingProfile {
  // Video settings
  crf: number;
  deblock: string;
  smartblur: boolean;
  deinterlace: boolean;
  resolution: number;
  psy_rd: number;
  psy_rdoq: number;
  aq_strength: number;
  hardsubs: boolean;

  // Multi-resolution encoding
  multiResolution: boolean;
  outputResolutions: number[]; // Array of resolutions to encode to (480, 720, 1080)

  // Advanced x265 params
  me: number;
  rd: number;
  subme: number;
  aq_mode: number;
  merange: number;
  bframes: number;
  b_adapt: number;
  limit_sao: boolean;
  frame_threads: number;

  // Output format
  format: string;
}

export interface EncodingProgress {
  percent: number;
  currentFile: string;
  speed: string;
  eta: string;
  completed: boolean;
}

export class Encoder {
  private ffmpegProcess: ChildProcess | null = null;
  private isEncoding = false;
  private progress: EncodingProgress = {
    percent: 0,
    currentFile: '',
    speed: 'N/A',
    eta: 'Calculating...',
    completed: false,
  };
  private totalDuration = 0;
  private totalFrames = 0;
  private profiles: Record<string, EncodingProfile> = {};
  private currentProfile: EncodingProfile | null = null;
  private ffmpegBinaryPath: string;
  private manualStop = false;

  constructor() {
    // Re-find the FFmpeg path when the encoder is created to ensure it's fresh
    this.ffmpegBinaryPath = findFFmpegPath();

    log.info('Initializing encoder');
    log.info('FFmpeg path:', this.ffmpegBinaryPath);

    // Display detailed information about the ffmpeg path
    try {
      const stats = fs.statSync(this.ffmpegBinaryPath);
      log.debug(
        `FFmpeg file exists: ${stats.isFile()}, size: ${stats.size} bytes`
      );
    } catch (error: unknown) {
      const errorMessage =
        error instanceof Error ? error.message : String(error);
      log.error(`Error checking FFmpeg binary: ${errorMessage}`);
    }

    if (!this.ffmpegBinaryPath) {
      log.error(
        'FFmpeg binary not found. Encoding functionality may not work properly.'
      );
    }
  }

  setProfiles(profiles: Record<string, EncodingProfile>): void {
    log.info('Setting encoder profiles:', Object.keys(profiles).join(', '));
    this.profiles = profiles;
  }

  getProgress(): EncodingProgress {
    return { ...this.progress };
  }

  isRunning(): boolean {
    return this.isEncoding;
  }

  stop(): void {
    if (this.ffmpegProcess && !this.ffmpegProcess.killed) {
      log.info('Stopping encoding process');

      // Set a flag to indicate this was a manual stop
      this.manualStop = true;

      this.ffmpegProcess.kill();
      this.isEncoding = false;
      this.progress.completed = true;
    } else {
      log.warn('Attempted to stop encoding, but no process was running');
    }
  }

  async encode(
    inputFile: string,
    outputDir: string,
    profileName: string
  ): Promise<void> {
    log.info('Encode request received', { inputFile, outputDir, profileName });

    if (this.isEncoding) {
      const error = 'Encoder is already running';
      log.error(error);
      throw new Error(error);
    }

    // Check if ffmpeg path exists
    if (!this.ffmpegBinaryPath) {
      const error =
        'FFmpeg binary not found. Please make sure ffmpeg is installed.';
      log.error(error);
      throw new Error(error);
    }

    // Check if input file exists
    if (!fs.existsSync(inputFile)) {
      const error = `Input file does not exist: ${inputFile}`;
      log.error(error);
      throw new Error(error);
    }

    // Get the selected encoding profile
    const profile = this.profiles[profileName];
    if (!profile) {
      const error = `Profile '${profileName}' not found`;
      log.error(error, { availableProfiles: Object.keys(this.profiles) });
      throw new Error(error);
    }
    this.currentProfile = profile;
    log.info('Using profile:', { profileName, profile });

    const outputFile = path.join(
      outputDir,
      `${path.parse(inputFile).name}.${profile.format}`
    );

    // Create output directory if it doesn't exist
    if (!fs.existsSync(outputDir)) {
      log.info(`Creating output directory: ${outputDir}`);
      fs.mkdirSync(outputDir, { recursive: true });
    }

    // Prepare ffmpeg arguments
    const args = this.buildFfmpegArgs(inputFile, outputFile, profile);
    log.debug('FFmpeg command:', `${this.ffmpegBinaryPath} ${args.join(' ')}`);

    log.info(
      `Starting encoding of ${path.basename(
        inputFile
      )} with profile ${profileName}`
    );
    log.info(`Output file: ${outputFile}`);
    log.info(`FFmpeg path: ${this.ffmpegBinaryPath}`);

    // Reset progress
    this.progress = {
      percent: 0,
      currentFile: path.basename(inputFile),
      speed: 'N/A',
      eta: 'Calculating...',
      completed: false,
    };

    return new Promise((resolve, reject) => {
      this.isEncoding = true;
      try {
        // Double check FFmpeg path right before using it
        if (
          !fs.existsSync(this.ffmpegBinaryPath) &&
          this.ffmpegBinaryPath !== 'ffmpeg'
        ) {
          log.warn(
            `FFmpeg binary not found at ${this.ffmpegBinaryPath}, falling back to system PATH`
          );
          this.ffmpegBinaryPath = 'ffmpeg';
        }

        log.debug('Spawning FFmpeg process using path:', this.ffmpegBinaryPath);
        this.ffmpegProcess = spawn(this.ffmpegBinaryPath, args);
        log.debug('FFmpeg process spawned', { pid: this.ffmpegProcess.pid });
      } catch (error) {
        this.isEncoding = false;
        log.error('Failed to spawn FFmpeg process:', error);
        reject(new Error(`Failed to spawn FFmpeg process: ${error}`));
        return;
      }

      // Handle stdout output
      this.ffmpegProcess.stdout?.on('data', (data) => {
        const output = data.toString().trim();
        if (output) {
          log.debug(`FFmpeg stdout: ${output}`);
        }
      });

      // Handle stderr output to parse progress
      this.ffmpegProcess.stderr?.on('data', (data) => {
        const output = data.toString();
        log.debug(`FFmpeg stderr: ${output}`);
        this.parseProgress(output);
      });

      this.ffmpegProcess.on('close', (code) => {
        this.isEncoding = false;
        this.ffmpegProcess = null;

        if (code === 0) {
          this.progress.percent = 100;
          this.progress.completed = true;
          log.info(`Encoding completed successfully: ${outputFile}`);
          resolve();
        } else {
          log.error(`FFmpeg exited with code ${code}`);
          reject(new Error(`FFmpeg exited with code ${code}`));
        }
      });

      this.ffmpegProcess.on('error', (err) => {
        this.isEncoding = false;
        this.ffmpegProcess = null;
        log.error('FFmpeg process error:', err);
        reject(err);
      });
    });
  }

  async encodeFiles(
    files: string[],
    outputDir: string,
    profileName: string
  ): Promise<void> {
    log.info(`Starting batch encoding of ${files.length} files`, {
      outputDir,
      profileName,
      files: files.map((f) => path.basename(f)),
    });

    for (const file of files) {
      try {
        await this.encode(file, outputDir, profileName);
      } catch (error) {
        log.error(`Error encoding file ${file}:`, error);
        throw error;
      }
    }

    log.info('Batch encoding completed successfully');
  }

  private parseProgress(output: string): void {
    // Parse duration if we don't have it yet
    if (!this.totalDuration) {
      const durationMatch = output.match(
        /Duration: (\d{2}):(\d{2}):(\d{2}.\d{2})/
      );
      if (durationMatch) {
        const hours = parseInt(durationMatch[1], 10);
        const minutes = parseInt(durationMatch[2], 10);
        const seconds = parseFloat(durationMatch[3]);
        this.totalDuration = hours * 3600 + minutes * 60 + seconds;
        log.info(
          `Video duration detected: ${hours}h ${minutes}m ${seconds}s (${this.totalDuration.toFixed(
            2
          )}s)`
        );
      }
    }

    // Look for frame information when time is N/A
    const frameMatch = output.match(/frame=\s*(\d+)/);
    const fpsMatch = output.match(/fps=\s*(\d+)/);
    
    // Check if we have total frames info in the output
    if (!this.totalFrames) {
      const totalFramesMatch = output.match(/NUMBER_OF_FRAMES-eng:\s*(\d+)/);
      if (totalFramesMatch) {
        this.totalFrames = parseInt(totalFramesMatch[1], 10);
        log.info(`Total frames detected: ${this.totalFrames}`);
      }
    }

    // Parse current progress based on frame count if time is not available
    if (frameMatch && this.totalFrames && this.totalFrames > 0) {
      const currentFrame = parseInt(frameMatch[1], 10);
      this.progress.percent = (currentFrame / this.totalFrames) * 100;
      
      // Update speed based on fps
      if (fpsMatch) {
        const fps = parseInt(fpsMatch[1], 10);
        this.progress.speed = `${fps} fps`;
        
        // Calculate ETA based on frames and fps
        if (fps > 0) {
          const remainingFrames = this.totalFrames - currentFrame;
          const remainingSeconds = remainingFrames / fps;
          const etaMinutes = Math.floor(remainingSeconds / 60);
          const etaSeconds = Math.floor(remainingSeconds % 60);
          this.progress.eta = `${etaMinutes}:${etaSeconds.toString().padStart(2, '0')}`;
        }
      }
    } else {
      // If we don't have frame-based progress, try time-based progress
      const timeMatch = output.match(/time=(\d{2}):(\d{2}):(\d{2}.\d{2})/);
      if (timeMatch && this.totalDuration) {
        const hours = parseInt(timeMatch[1], 10);
        const minutes = parseInt(timeMatch[2], 10);
        const seconds = parseFloat(timeMatch[3]);
        const currentTime = hours * 3600 + minutes * 60 + seconds;
        this.progress.percent = (currentTime / this.totalDuration) * 100;
        
        // Get speed info
        const speedMatch = output.match(/speed=(\d+\.\d+)x/);
        if (speedMatch) {
          this.progress.speed = `${speedMatch[1]}x`;
          
          // Calculate ETA based on time and speed
          const speedValue = parseFloat(speedMatch[1]);
          if (speedValue > 0) {
            const remainingSeconds = 
              (this.totalDuration - currentTime) / speedValue;
            const etaMinutes = Math.floor(remainingSeconds / 60);
            const etaSeconds = Math.floor(remainingSeconds % 60);
            this.progress.eta = `${etaMinutes}:${etaSeconds.toString().padStart(2, '0')}`;
          }
        }
      }
    }
  }

  private buildFfmpegArgs(
    inputFile: string,
    outputFile: string,
    profile: EncodingProfile
  ): string[] {
    log.debug('Building FFmpeg arguments', { inputFile, outputFile, profile });

    const vfFilters = [];
    let audioBitrate: string;
    let audioQuality: number;
    let actualCrf = profile.crf;

    // Add resolution scaling with proper syntax for FFmpeg CLI
    switch (profile.resolution) {
      case 480:
        vfFilters.push(
          'scale=848:480:flags=spline+accurate_rnd+full_chroma_int'
        );
        audioBitrate = '96k';
        audioQuality = 1.1;
        break;
      case 720:
        vfFilters.push(
          'scale=1280:720:flags=spline+accurate_rnd+full_chroma_int'
        );
        audioBitrate = '160k';
        audioQuality = 1.4;
        break;
      case 1080:
        vfFilters.push(
          'scale=1920:1080:flags=spline+accurate_rnd+full_chroma_int'
        );
        audioBitrate = '192k';
        audioQuality = 1.8;
        break;
      default:
        // Fallback to 720p if resolution is not recognized
        vfFilters.push(
          'scale=1280:720:flags=spline+accurate_rnd+full_chroma_int'
        );
        audioBitrate = '160k';
        audioQuality = 1.4;
    }

    // Add additional filters
    if (profile.smartblur) {
      vfFilters.push('smartblur=1.5:-0.35:-3.5:0.65:0.25:2.0');
    }

    if (profile.deinterlace) {
      vfFilters.push('yadif=1');
    }

    // Handle subtitles if needed
    if (profile.format === 'mp4' && profile.hardsubs) {
      const subtitlesPath = inputFile.replace(/\\/g, '/');
      vfFilters.push(`subtitles='${subtitlesPath}'`);
    }

    // CPU optimization - Calculate optimal thread count based on system
    const coreCount = require('os').cpus().length;
    // Use 75% of available cores for encoding to avoid system slowdown
    const threadCount = Math.max(2, Math.floor(coreCount * 0.75));

    // Build x265 params with optimized settings for CPU encoding
    const x265Params = [
      `me=${profile.me}`,
      `rd=${profile.rd}`,
      `subme=${profile.subme}`,
      `aq-mode=${profile.aq_mode}`,
      `aq-strength=${profile.aq_strength}`,
      `deblock=${profile.deblock}`,
      `psy-rd=${profile.psy_rd}`,
      `psy-rdoq=${profile.psy_rdoq}`,
      'rdoq-level=2',
      `merange=${profile.merange}`,
      `bframes=${profile.bframes}`,
      `b-adapt=${profile.b_adapt}`,
      profile.limit_sao ? 'limit-sao=1' : 'limit-sao=0',
      // Use optimal thread configuration
      `frame-threads=${profile.frame_threads}`,
      `pools=+,-`, // Use all CPU cores efficiently
      'no-info=1',
    ];

    const args = [
      // Multi-threading optimization
      '-threads',
      threadCount.toString(),

      '-i',
      inputFile,
      // Copy all metadata including chapters
      '-map_metadata',
      '0',
      '-map_chapters',
      '0',
      // Map streams
      '-map',
      '0:v', // Map video stream(s)
      '-map',
      '0:a', // Map audio stream(s)
      '-c:v',
      'libx265',
      '-c:a',
      'aac',
      '-b:a',
      audioBitrate,
      '-q:a',
      audioQuality.toString(),
      '-profile:v',
      'main',
      '-crf',
      actualCrf.toString(),
      // Use faster preset for better speed
      '-preset',
      'medium',
      '-pix_fmt',
      'yuv420p',
      '-vf',
      vfFilters.join(','),
      '-color_range',
      '1',
      '-color_primaries',
      '1',
      '-colorspace',
      '1',
      '-color_trc',
      '1',
      '-x265-params',
      x265Params.join(':'),
    ];

    // Format specific settings
    if (profile.format === 'mkv') {
      args.push('-f', 'matroska');
      // Map all subtitle streams
      args.push('-map', '0:s?');
      // Preserve attachments (fonts, etc.) if present
      args.push('-map', '0:t?');
    } else {
      args.push('-f', 'mp4');
      // MP4 format doesn't support as many subtitle formats as MKV
      // But we'll still try to copy compatible subtitle formats
      args.push('-map', '0:s?');
      args.push('-c:s', 'mov_text');
    }

    args.push('-y', outputFile);

    return args;
  }
}
