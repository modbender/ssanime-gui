# Build Resources

This directory contains build resources for electron-builder.

## Icons

The following icon files are included:

- `icon.png` (512x512) - Base icon for Linux
- `icon.ico` - Windows icon file
- `icon.icns` - macOS icon file (to be generated)

### Current Status

- ✅ **Windows**: `icon.ico` - Properly formatted icon created from PNG
- ✅ **Linux**: `icon.png` - High-quality 512x512 PNG icon
- ✅ **macOS**: `icon.icns` - Basic placeholder icon (functional but simple)

### Generating Better Icons

To create better icons from the `public/logo.svg`:

1. **Online Conversion Tools**:

   - https://iconverticons.com/online/
   - https://convertio.co/svg-icns/
   - https://convertio.co/svg-ico/

2. **Command Line Tools**:

   ```bash
   # Install ImageMagick or similar tools
   # Convert SVG to different formats
   convert public/logo.svg -resize 512x512 build/icon.png
   ```

3. **macOS ICNS Generation**:

   ```bash
   # On macOS, use iconutil
   mkdir icon.iconset
   sips -z 16 16 icon.png --out icon.iconset/icon_16x16.png
   # ... (repeat for other sizes)
   iconutil -c icns icon.iconset
   ```

4. **Project Scripts**:

   ```bash
   # Regenerate all icons using project scripts
   pnpm icons:create

   # Generate individual icon types
   pnpm icons:icns  # Create macOS ICNS
   pnpm icons:ico   # Create Windows ICO (requires png-to-ico package)
   ```

The current placeholder icons will allow the build to complete successfully.
