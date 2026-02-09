import { execSync } from 'child_process'
import * as fs from 'fs'
import * as path from 'path'

const BACKEND_DIR = path.resolve(__dirname, '../../backend')
const PID_FILE = path.resolve(__dirname, '.backend-pid')

function log(msg: string) {
  console.log(`[e2e-teardown] ${msg}`)
}

export default async function globalTeardown() {
  log('Starting E2E global teardown...')

  // 1. Kill the backend process
  if (fs.existsSync(PID_FILE)) {
    const pid = parseInt(fs.readFileSync(PID_FILE, 'utf-8').trim(), 10)
    log(`Killing backend process (PID: ${pid})...`)
    try {
      // Kill the process group (negative PID) since we used detached: true
      process.kill(-pid, 'SIGTERM')
    } catch {
      // Process may already be dead
      log('Backend process already stopped.')
    }
    fs.unlinkSync(PID_FILE)
  }

  // 2. Tear down the database container
  log('Stopping E2E database...')
  try {
    execSync('docker compose -f docker-compose.e2e.yml down', {
      cwd: BACKEND_DIR,
      stdio: 'inherit',
    })
  } catch {
    log('Warning: Failed to stop database containers.')
  }

  log('Global teardown complete!')
}
