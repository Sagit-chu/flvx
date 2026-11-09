import type {
  SystemUpgradeJob,
  SystemUpgradeRunApiData,
  SystemUpgradeStatusApiData,
} from "@/api/types";

type Response<T> = {
  code: number;
  data: T;
  msg?: string;
  transportError?: boolean;
};
export interface UpgradeMonitorState {
  job: SystemUpgradeJob | null;
  currentVersion: string;
  checking: boolean;
  submitting: boolean;
  uncertain: boolean;
  paused: boolean;
  notice: string;
}

export const initialUpgradeState: UpgradeMonitorState = {
  job: null,
  currentVersion: "",
  checking: true,
  submitting: false,
  uncertain: true,
  paused: false,
  notice: "",
};

export const upgradeIsBlocked = (state: UpgradeMonitorState) =>
  state.checking ||
  state.submitting ||
  state.uncertain ||
  Boolean(
    state.job?.active ||
      state.job?.requiresRecovery ||
      state.job?.stage === "recovery_required",
  );

// The server supplies sanitized public summaries, never helper logs. Render as
// text only; remove control characters and cap length as defense in depth.
export const upgradeMessage = (message: string) =>
  message.replace(/[\u0000-\u001f\u007f]/g, " ").slice(0, 500);

// Kept independent of React so lifecycle/disconnect races can be replayed
// without adding frontend test infrastructure.
export function createUpgradeMonitor(
  api: {
    status: (id?: string) => Promise<Response<SystemUpgradeStatusApiData>>;
    run: (
      version?: string,
      channel?: "stable" | "dev",
    ) => Promise<Response<SystemUpgradeRunApiData>>;
  },
  publish: (state: UpgradeMonitorState) => void,
  clock = {
    now: () => Date.now(),
    schedule: (fn: () => void, ms: number) => setTimeout(fn, ms),
    cancel: (timer: ReturnType<typeof setTimeout>) => clearTimeout(timer),
  },
) {
  let state = { ...initialUpgradeState };
  let timer: ReturnType<typeof setTimeout> | undefined;
  let generation = 0;
  let disposed = false;
  let busy = false;
  let deadline = 0;
  let failures = 0;
  let expectedJob: string | undefined;
  let awaitingSubmission = false;
  let previousJob: string | undefined;
  const emit = (patch: Partial<UpgradeMonitorState>) => {
    state = { ...state, ...patch };
    if (!disposed) publish(state);
  };
  const cancelTimer = () => {
    if (timer !== undefined) clock.cancel(timer);
    timer = undefined;
  };
  const poll = async () => {
    if (disposed || busy) return;
    busy = true;
    const request = generation;

    try {
      const response = await api.status(expectedJob);

      if (disposed || request !== generation) return;
      if (response.code !== 0 || !response.data) throw new Error("unavailable");
      const { job, currentVersion } = response.data;

      // An accepted or interrupted POST must not be mistaken for an idle
      // server, nor may an older completed job certify the new request.
      if (
        (expectedJob && job?.id !== expectedJob) ||
        (awaitingSubmission && (!job || job.id === previousJob))
      ) {
        throw new Error("awaiting job");
      }
      failures = 0;
      awaitingSubmission = false;
      expectedJob = job?.id;
      emit({
        job,
        currentVersion,
        checking: false,
        uncertain: false,
        paused: false,
      });
    } catch {
      if (disposed || request !== generation) return;
      failures += 1;
      // Disconnection is not proof of failure. Keep the last known job and
      // continue blocking submissions until authoritative status is available.
      emit({ checking: false, uncertain: true });
    } finally {
      busy = false;
      if (
        !disposed &&
        request === generation &&
        (state.uncertain || state.job?.active)
      ) {
        if (clock.now() >= deadline) {
          emit({ paused: true });
        } else {
          timer = clock.schedule(
            () => {
              void poll();
            },
            Math.min(15000, 2000 * 2 ** Math.min(failures, 3)),
          );
        }
      }
    }
  };

  return {
    refresh() {
      if (disposed || busy || state.submitting) return;
      cancelTimer();
      if (
        !state.uncertain &&
        !state.job?.active &&
        !state.job?.requiresRecovery
      ) {
        expectedJob = undefined;
      }
      deadline = clock.now() + 15 * 60 * 1000;
      failures = 0;
      emit({ checking: true, paused: false });
      void poll();
    },
    async submit(version?: string, channel?: "stable" | "dev") {
      if (disposed || busy || upgradeIsBlocked(state)) return false;
      cancelTimer();
      const request = ++generation;

      previousJob = state.job?.id;
      expectedJob = undefined;
      awaitingSubmission = true;
      emit({
        job: null,
        submitting: true,
        uncertain: true,
        paused: false,
        notice: "",
      });
      try {
        const response = await api.run(version, channel);

        if (disposed || request !== generation) return false;
        if (response.code === 0 && response.data?.jobId) {
          expectedJob = response.data.jobId;
          awaitingSubmission = false;
        } else if (response.code !== 0 && !response.transportError) {
          // Definite server rejection: rediscover the authoritative job,
          // including concurrent submissions from other admin sessions.
          awaitingSubmission = false;
          emit({
            notice: upgradeMessage(
              response.msg || "升级请求被拒绝，请检查当前任务状态。",
            ),
          });
        }
      } catch {
        // The POST may already have started the helper. Reconcile, don't retry.
      } finally {
        if (!disposed && request === generation) {
          emit({ submitting: false });
          deadline = clock.now() + 15 * 60 * 1000;
          void poll();
        }
      }

      return !disposed && request === generation;
    },
    dispose() {
      disposed = true;
      generation += 1;
      cancelTimer();
    },
  };
}
