import { lockAppScroll } from '../utils/scrollLock.js';

const getDocumentTarget = () => (typeof document === 'undefined' ? null : document);

const canRestoreFocus = (documentTarget, element) => Boolean(
  element
  && typeof element.focus === 'function'
  && documentTarget?.contains(element)
);

export const createModalLifecycleManager = () => {
  const stack = [];

  return {
    activate({ getOnClose, initialFocusElement }) {
      const documentTarget = getDocumentTarget();
      const previousFocus = documentTarget?.activeElement || null;
      const unlockScroll = lockAppScroll();
      const entry = { getOnClose, initialFocusElement, previousFocus };
      let active = true;

      stack.push(entry);

      const handleKeyDown = (event) => {
        if (event.key !== 'Escape') return;
        if (stack[stack.length - 1] !== entry) return;
        getOnClose?.()?.();
      };

      documentTarget?.addEventListener('keydown', handleKeyDown);
      initialFocusElement?.focus?.();

      return () => {
        if (!active) return;
        active = false;

        documentTarget?.removeEventListener('keydown', handleKeyDown);
        const index = stack.indexOf(entry);
        const wasTopmost = index === stack.length - 1;
        const nextEntry = index >= 0 ? stack[index + 1] : null;
        if (nextEntry && !canRestoreFocus(documentTarget, nextEntry.previousFocus)) {
          nextEntry.previousFocus = entry.previousFocus;
        }
        if (index >= 0) stack.splice(index, 1);
        unlockScroll();

        if (wasTopmost && canRestoreFocus(documentTarget, entry.previousFocus)) {
          entry.previousFocus.focus();
        }
      };
    }
  };
};

export const modalLifecycleManager = createModalLifecycleManager();
