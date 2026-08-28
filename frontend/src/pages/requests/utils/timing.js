export const TIMING_THRESHOLDS_MS = Object.freeze({
  first: {
    warning: 8000,
    critical: 15000
  },
  duration: {
    warning: 30000,
    critical: 60000
  }
});

const toSafeMs = (ms) => (Number.isFinite(ms) ? Math.max(ms, 0) : 0);

export const formatTimingBadge = (ms) => `${(toSafeMs(ms) / 1000).toFixed(1)}s`;

export const formatOptionalTimingBadge = (ms) => (
  Number.isFinite(ms) ? formatTimingBadge(ms) : '-'
);

export const calculateGenerationMs = (durationMs, firstTokenMs) => {
  if (!Number.isFinite(durationMs) || !Number.isFinite(firstTokenMs)) {
    return null;
  }
  return Math.max(durationMs - firstTokenMs, 0);
};

export const resolveFirstResponseMs = (firstTokenMs, durationMs, isStreaming) => {
  if (Number.isFinite(firstTokenMs)) {
    return Math.max(firstTokenMs, 0);
  }
  if (!isStreaming && Number.isFinite(durationMs)) {
    return Math.max(durationMs, 0);
  }
  return null;
};

export const resolveCompletionMs = (completionMs, durationMs, firstTokenMs, isStreaming = true) => (
  Number.isFinite(completionMs)
    ? Math.max(completionMs, 0)
    : (!isStreaming && Number.isFinite(firstTokenMs)
      ? 0
      : calculateGenerationMs(durationMs, firstTokenMs))
);

export const calculateTokensPerSecond = (outputTokens, generationMs) => {
  if (!Number.isFinite(outputTokens) || !Number.isFinite(generationMs) || generationMs <= 0) {
    return null;
  }
  return Math.max(outputTokens, 0) / (generationMs / 1000);
};

export const formatTpsBadge = (tokensPerSecond) => {
  if (!Number.isFinite(tokensPerSecond)) {
    return '-';
  }
  return tokensPerSecond >= 100
    ? tokensPerSecond.toFixed(0)
    : tokensPerSecond.toFixed(1);
};

export const getTimingPillClassName = (type, ms) => {
  const value = toSafeMs(ms);
  const thresholds = type === 'first'
    ? TIMING_THRESHOLDS_MS.first
    : TIMING_THRESHOLDS_MS.duration;

  if (value > thresholds.critical) {
    return 'tone-rose';
  }
  if (value > thresholds.warning) {
    return type === 'first' ? 'tone-amber' : 'tone-orange';
  }
  return 'tone-emerald';
};
