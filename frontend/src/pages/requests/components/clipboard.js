function legacyCopyText(text) {
  if (typeof document === 'undefined') {
    return false;
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  textarea.style.pointerEvents = 'none';

  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();

  let copied = false;
  try {
    copied = document.execCommand('copy');
  } catch {
    copied = false;
  }

  document.body.removeChild(textarea);
  return copied;
}

export async function copyTextToClipboard(value, label = '内容') {
  if (value == null || value === '') {
    return false;
  }

  const text = String(value);

  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch (error) {
      console.warn(`复制${label}失败，尝试降级复制`, error);
    }
  }

  if (legacyCopyText(text)) {
    return true;
  }

  if (typeof window !== 'undefined' && typeof window.prompt === 'function') {
    window.prompt(`复制失败，请手动复制${label}：`, text);
  }

  return false;
}
