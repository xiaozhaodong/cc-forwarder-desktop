const APP_SCROLL_CONTAINER_ID = 'app-scroll-container';

const getScrollLockTarget = () => {
  if (typeof document === 'undefined') {
    return null;
  }

  return document.getElementById(APP_SCROLL_CONTAINER_ID) || document.body;
};

export const lockAppScroll = () => {
  const target = getScrollLockTarget();
  if (!target) {
    return () => {};
  }

  const currentCount = Number(target.dataset.scrollLockCount || '0');
  if (currentCount === 0) {
    target.dataset.scrollLockOverflow = target.style.overflow || '';
  }

  target.dataset.scrollLockCount = String(currentCount + 1);
  target.style.overflow = 'hidden';

  return () => {
    const nextCount = Math.max(Number(target.dataset.scrollLockCount || '1') - 1, 0);
    if (nextCount === 0) {
      target.style.overflow = target.dataset.scrollLockOverflow || '';
      delete target.dataset.scrollLockCount;
      delete target.dataset.scrollLockOverflow;
      return;
    }

    target.dataset.scrollLockCount = String(nextCount);
  };
};

