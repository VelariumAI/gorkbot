export class UniversalClipboard {
  static async copyToClipboard(text) {
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(text);
        return true;
      } catch (err) {
        console.warn('Clipboard API failed, falling back...', err);
      }
    }
    return this.fallbackCopyTextToClipboard(text);
  }

  static fallbackCopyTextToClipboard(text) {
    const textArea = document.createElement("textarea");
    textArea.value = text;
    textArea.style.top = "0";
    textArea.style.left = "0";
    textArea.style.position = "fixed";
    textArea.style.opacity = "0";
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();

    try {
      const successful = document.execCommand('copy');
      document.body.removeChild(textArea);
      return successful;
    } catch (err) {
      console.error('Fallback: Oops, unable to copy', err);
      document.body.removeChild(textArea);
      return false;
    }
  }

  static initDelegation() {
    document.body.addEventListener('click', async (e) => {
      const btn = e.target.closest('.copy-btn');
      if (!btn) return;

      let textToCopy = '';
      if (btn.classList.contains('macro-copy')) {
        // Find closest message container
        const container = btn.closest('.message.system').querySelector('.content');
        textToCopy = container.innerText; // Preserves formatting/line breaks
      } else if (btn.classList.contains('micro-copy')) {
        const pre = btn.closest('.code-container').querySelector('code');
        textToCopy = pre.innerText;
      }

      if (textToCopy) {
        const success = await this.copyToClipboard(textToCopy);
        if (success) {
          const originalHTML = btn.innerHTML;
          const originalLabel = btn.getAttribute('aria-label');
          btn.innerHTML = '<i class="fa-solid fa-check" style="color:var(--success)"></i>';
          btn.setAttribute('aria-label', 'Copied!');
          setTimeout(() => {
            btn.innerHTML = originalHTML;
            btn.setAttribute('aria-label', originalLabel);
          }, 2000);
        }
      }
    });
  }

  static wrapCodeBlocks() {
    document.querySelectorAll('pre').forEach(pre => {
      if (pre.parentElement.classList.contains('code-container')) return;
      
      const wrapper = document.createElement('div');
      wrapper.className = 'code-container';
      wrapper.style.position = 'relative';
      
      pre.parentNode.insertBefore(wrapper, pre);
      wrapper.appendChild(pre);

      const btn = document.createElement('button');
      btn.className = 'copy-btn micro-copy btn-icon';
      btn.innerHTML = '<i class="fa-regular fa-copy"></i>';
      btn.setAttribute('aria-label', 'Copy code to clipboard');
      btn.style.position = 'absolute';
      btn.style.top = '0.5rem';
      btn.style.right = '0.5rem';
      btn.style.background = 'rgba(0,0,0,0.5)';
      btn.style.border = 'none';
      btn.style.borderRadius = '4px';
      btn.style.padding = '4px';
      btn.style.color = '#fff';
      btn.style.cursor = 'pointer';
      
      wrapper.appendChild(btn);
    });
  }
}
