(() => {
  const buttons = document.querySelectorAll("[data-copy-button]");

  function copyWithFallback(text) {
    const textArea = document.createElement("textarea");
    textArea.value = text;
    textArea.setAttribute("readonly", "");
    textArea.style.position = "fixed";
    textArea.style.opacity = "0";
    document.body.appendChild(textArea);
    textArea.select();
    const copied = document.execCommand("copy");
    textArea.remove();

    if (!copied) {
      throw new Error("The browser could not copy the command.");
    }
  }

  async function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text);
      return;
    }

    copyWithFallback(text);
  }

  buttons.forEach((button) => {
    const block = button.closest(".code-block");
    const code = block?.querySelector("code");
    const status = block?.querySelector("[data-copy-status]");
    if (!code || !status) {
      return;
    }

    const initialLabel = button.textContent;
    const initialStatus = status.textContent;

    button.addEventListener("click", async () => {
      button.disabled = true;
      const text = code.textContent.trim();

      try {
        await copyText(text);
        button.textContent = "Copied";
        status.textContent = "Command copied to clipboard.";
      } catch {
        button.textContent = "Try again";
        status.textContent = "Copy failed. Select the command and copy it manually.";
      } finally {
        window.setTimeout(() => {
          button.textContent = initialLabel;
          button.disabled = false;
          status.textContent = initialStatus;
        }, 1800);
      }
    });
  });
})();
