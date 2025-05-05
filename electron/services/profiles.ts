import { app } from 'electron';
import * as fs from 'fs';
import * as path from 'path';
import type { EncodingProfile } from './encoder';

export class ProfileManager {
  private profilesPath: string;
  private profiles: Record<string, EncodingProfile>;
  private defaultProfileNames = ['SSAnime MKV', 'SSAnime MP4'];

  constructor() {
    try {
      // Ensure userData directory exists
      const userDataPath = app.getPath('userData');
      if (!fs.existsSync(userDataPath)) {
        fs.mkdirSync(userDataPath, { recursive: true });
      }

      this.profilesPath = path.join(userDataPath, 'encoding-profiles.json');
      this.profiles = this.loadDefaultProfiles();

      // Load saved profiles if they exist
      this.loadProfiles();
    } catch (error) {
      console.error('Error initializing ProfileManager:', error);
      // Fallback to memory-only profiles if file system access fails
      this.profilesPath = '';
      this.profiles = this.loadDefaultProfiles();
    }
  }

  getProfiles(): Record<string, EncodingProfile> {
    return { ...this.profiles };
  }

  saveProfiles(profiles: Record<string, EncodingProfile>): void {
    // Make sure we always have the default profiles
    for (const defaultName of this.defaultProfileNames) {
      if (!profiles[defaultName]) {
        profiles[defaultName] = this.loadDefaultProfiles()[defaultName];
      }
    }

    this.profiles = profiles;

    // Skip saving if profilesPath is empty (indicating file system access issues)
    if (!this.profilesPath) {
      console.warn('Skipping profile save due to file system access issues');
      return;
    }

    try {
      // Create backup of existing profiles before overwriting
      if (fs.existsSync(this.profilesPath)) {
        const backupPath = `${this.profilesPath}.bak`;
        fs.copyFileSync(this.profilesPath, backupPath);
      }

      // Write new profiles
      fs.writeFileSync(
        this.profilesPath,
        JSON.stringify(this.profiles, null, 2)
      );
    } catch (error) {
      console.error('Failed to save profiles:', error);
      throw new Error(
        `Failed to save profiles: ${
          error instanceof Error ? error.message : String(error)
        }`
      );
    }
  }

  isDefaultProfile(profileName: string): boolean {
    return this.defaultProfileNames.includes(profileName);
  }

  private loadProfiles(): void {
    // Skip loading if profilesPath is empty
    if (!this.profilesPath) return;

    try {
      if (fs.existsSync(this.profilesPath)) {
        const data = fs.readFileSync(this.profilesPath, 'utf-8');

        try {
          const savedProfiles = JSON.parse(data);

          // Validate profile structure before merging
          for (const [name, profile] of Object.entries(savedProfiles)) {
            this.validateProfile(profile as Partial<EncodingProfile>);
          }

          // Merge with default profiles, ensuring defaults are always present
          this.profiles = {
            ...this.loadDefaultProfiles(),
            ...savedProfiles,
          };

          // Make sure default profiles are not missing
          for (const defaultName of this.defaultProfileNames) {
            if (!this.profiles[defaultName]) {
              this.profiles[defaultName] =
                this.loadDefaultProfiles()[defaultName];
            }
          }
        } catch (parseError) {
          console.error('Failed to parse profiles JSON:', parseError);
          // If JSON parsing fails, use default profiles but don't overwrite the corrupted file
        }
      }
    } catch (error) {
      console.error('Failed to load profiles:', error);
    }
  }

  // Validate profile to ensure required properties exist
  private validateProfile(profile: Partial<EncodingProfile>): void {
    const requiredProps = ['crf', 'resolution', 'format'];

    for (const prop of requiredProps) {
      if (profile[prop as keyof EncodingProfile] === undefined) {
        throw new Error(`Invalid profile: missing required property '${prop}'`);
      }
    }
  }

  private loadDefaultProfiles(): Record<string, EncodingProfile> {
    // Base settings common for both profiles
    const baseSettings = {
      // Video settings
      crf: 23,
      deblock: '0:0',
      smartblur: false,
      deinterlace: false,
      resolution: 720,
      psy_rd: 1.0,
      psy_rdoq: 1.0,
      aq_strength: 1.0,

      // Multi-resolution encoding
      multiResolution: false,
      outputResolutions: [720],

      // Advanced x265 params
      me: 2,
      rd: 4,
      subme: 7,
      aq_mode: 3,
      merange: 57,
      bframes: 8,
      b_adapt: 2,
      limit_sao: true,
      frame_threads: 3,
    };

    return {
      'SSAnime MKV': {
        ...baseSettings,
        hardsubs: false,
        format: 'mkv',
      },
      'SSAnime MP4': {
        ...baseSettings,
        hardsubs: true,
        format: 'mp4',
      },
    };
  }
}
