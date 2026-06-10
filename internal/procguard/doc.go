// Package procguard ties spawned child processes to the daemon's lifetime so a
// force-kill of the daemon does not orphan them.
//
// The daemon spawns ffmpeg/ffprobe children. If the daemon is force-killed
// (Stop-Process -Force / SIGKILL) there is no graceful shutdown, deferred
// cleanup, or context cancel to run, so without OS-level enforcement the
// children survive and keep holding file handles ("Device or resource busy").
//
// On Windows the daemon owns a job object created with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE and assigns every child to it. The OS
// closes the job handle when the daemon's process exits for any reason, which
// fires the limit and reaps the children. On other platforms there is currently
// no equivalent and Reap is a no-op.
package procguard
