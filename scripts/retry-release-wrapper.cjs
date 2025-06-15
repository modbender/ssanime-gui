const { execSync } = require('child_process');
const path = require('path');

const args = process.argv.slice(2);

if (!args[0]) {
    console.log('[ERROR] Usage: pnpm retry-release <version> [force]');
    process.exit(1);
}

const isWin = process.platform === 'win32';
const scriptPath = isWin ? 
    path.join(__dirname, 'retry-release.bat') : 
    path.join(__dirname, 'retry-release.sh');

const cmd = isWin ? 
    `"${scriptPath}" ${args.join(' ')}` : 
    `bash "${scriptPath}" ${args.join(' ')}`;

try {
    execSync(cmd, { stdio: 'inherit' });
} catch (error) {
    process.exit(error.status || 1);
}
