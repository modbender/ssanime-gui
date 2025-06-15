import pngToIco from 'png-to-ico';
import fs from 'fs';

// Create a proper ICO file from the PNG
pngToIco('build/icon.png')
  .then(buf => {
    fs.writeFileSync('build/icon.ico', buf);
    console.log('Created proper icon.ico file');
  })
  .catch(err => {
    console.error('Error creating ICO file:', err);
  });
