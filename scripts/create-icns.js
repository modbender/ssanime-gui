import fs from 'fs';

// Create a simple 1024-byte ICNS header with minimal icon data
// This is a basic ICNS file structure that should work for builds
const icnsHeader = Buffer.from([
  0x69,
  0x63,
  0x6e,
  0x73, // 'icns' magic
  0x00,
  0x00,
  0x04,
  0x00, // file size (1024 bytes)
]);

// Create padding to make it 1024 bytes
const padding = Buffer.alloc(1024 - icnsHeader.length);

// Combine header and padding
const icnsData = Buffer.concat([icnsHeader, padding]);

// Write the ICNS file
fs.writeFileSync('build/icon.icns', icnsData);

console.log('Created placeholder icon.icns file');
