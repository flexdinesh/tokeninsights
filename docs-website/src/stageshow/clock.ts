// The stage clock. Everything on a stage moves on the browser's own clocks:
// performance.now, requestAnimationFrame, setTimeout/setInterval, and CSS
// transitions and animations on the compositor. With nothing attached, that
// is all this file is: `clock` reads those same globals, so a stage plays in
// real time exactly as it always has.
//
// A recorder (film.mjs) installs a virtual clock in the page before any of
// its code loads; this vendored copy only reads it. From then on time only moves when the
// recorder steps it, by exactly 1/fps: timers fire in time order, one
// animation frame runs, and every CSS transition and animation is set to the
// new time. Live playback instead calls advance() with the native RAF's
// elapsed time, so its motion follows the display refresh rate. Each fixed
// frame of a video is then the same on every render, a
// timed beat lands on a known frame, and nothing drops or doubles.
//
// Chrome's own virtual time (Emulation.setVirtualTimePolicy) doesn't do this:
// it moves timers but runs no animation frames and leaves CSS animations
// where they are.

/** What a recorder drives, at window.__stageshow.clock once installed. */
export interface ClockControl {
  readonly fps: number
  /** Render updates since install; fixed frames when driven by step(). */
  readonly frame: number
  /** Virtual ms, what performance.now() returns. */
  now(): number
  /**
   * Advance `frames` frames (1 by default). Per frame: timers due by the new
   * time fire in order, each followed by the work it queued (React commits,
   * promise chains); then one round of animation frames; then every CSS
   * transition and animation is moved to the new time.
   */
  step(frames?: number): Promise<void>
  /** Advance live playback by elapsed milliseconds, rendering one update. */
  advance(milliseconds: number): Promise<void>
  /** Let queued tasks run without moving time, and hold any CSS animation that started meanwhile. */
  settle(): Promise<void>
  /** Stage measurement callbacks, separate from motion frames. */
  requestLayoutFrame(callback: FrameRequestCallback): number
  cancelLayoutFrame(id: number): void
  readonly layoutPending: boolean
  /** Run one round of layout callbacks without advancing time, timers or motion. */
  flushLayout(): Promise<void>
}

declare global {
  interface Window {
    /** Set by a recorder (film.mjs): its virtual clock, a slowdown and a forced theme. */
    __stageshow?: {
      clock?: ClockControl
      slow?: number
      theme?: 'light' | 'dark' | 'blueprint' | 'glass'
    }
  }
}

/**
 * The clock the kit and its scenes read. Each call goes to the page's global
 * at call time, so under a recorder it is the virtual clock and otherwise
 * the real one.
 */
export const clock = {
  now: (): number => performance.now(),
  raf: (fn: FrameRequestCallback): number => requestAnimationFrame(fn),
  cancelRaf: (id: number): void => cancelAnimationFrame(id),
  // Layout must finish even when a preview is paused. Keep these callbacks
  // separate: running arbitrary motion RAFs at a frozen timestamp can still
  // advance a spring that integrates once per callback.
  layoutRaf: (fn: FrameRequestCallback): number =>
    clock.recording?.requestLayoutFrame(fn) ?? requestAnimationFrame(fn),
  cancelLayoutRaf: (id: number): void => {
    const recording = clock.recording
    if (recording) recording.cancelLayoutFrame(id)
    else cancelAnimationFrame(id)
  },
  timeout: (fn: () => void, ms: number): number => window.setTimeout(fn, ms),
  clearTimeout: (id: number): void => window.clearTimeout(id),
  /** The recorder's control while one drives the page, else null. */
  get recording(): ClockControl | null {
    return typeof window === 'undefined' ? null : (window.__stageshow?.clock ?? null)
  },
}
