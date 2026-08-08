(function () {
  'use strict';

  /**
   * showCopyStatus updates the short status text next to the copy button.
   * statusEl is the element that displays feedback.
   * text is the message to show (empty clears the status).
   * isError marks the message as a failure when true.
   */
  function showCopyStatus(statusEl, text, isError) {
    if (!statusEl) {
      return;
    }
    statusEl.textContent = text;
    statusEl.classList.toggle('text-danger', Boolean(isError));
    statusEl.classList.toggle('text-success', Boolean(text) && !isError);
  }

  /**
   * copyText writes text into the system clipboard.
   * text is the diagnostics JSON string to copy.
   */
  async function copyText(text) {
    if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
      await navigator.clipboard.writeText(text);
      return;
    }

    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.setAttribute('readonly', '');
    textarea.style.position = 'absolute';
    textarea.style.left = '-9999px';
    document.body.appendChild(textarea);
    textarea.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(textarea);
    if (!ok) {
      throw new Error('Clipboard copy is not available');
    }
  }

  document.querySelectorAll('[data-info-diagnostics]').forEach((root) => {
    const button = root.querySelector('[data-info-diagnostics-copy]');
    const jsonEl = root.querySelector('[data-info-diagnostics-json]');
    const statusEl = root.querySelector('[data-info-diagnostics-copy-status]');
    if (!button || !jsonEl) {
      return;
    }

    button.addEventListener('click', async () => {
      const text = jsonEl.textContent || '';
      try {
        await copyText(text);
        showCopyStatus(statusEl, 'Copied', false);
      } catch (err) {
        showCopyStatus(statusEl, err.message || 'Copy failed', true);
      }
    });
  });
})();
