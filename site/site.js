(() => {
  const copyButtons = document.querySelectorAll("[data-copy-button]");
  const platformTabs = document.querySelectorAll("[data-platform-tab]");
  const platformPanels = document.querySelectorAll("[data-platform-panel]");

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

  copyButtons.forEach((button) => {
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

  function setPlatform(platform, moveFocus = false) {
    platformTabs.forEach((tab) => {
      const isActive = tab.dataset.platformTab === platform;
      tab.setAttribute("aria-selected", String(isActive));
      if (isActive && moveFocus) {
        tab.focus();
      }
    });

    platformPanels.forEach((panel) => {
      panel.hidden = panel.dataset.platformPanel !== platform;
    });
  }

  if (platformTabs.length && platformPanels.length) {
    setPlatform("linux");

    platformTabs.forEach((tab, index) => {
      tab.addEventListener("click", () => {
        setPlatform(tab.dataset.platformTab);
      });

      tab.addEventListener("keydown", (event) => {
        if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") {
          return;
        }

        event.preventDefault();
        const offset = event.key === "ArrowRight" ? 1 : -1;
        const nextIndex = (index + offset + platformTabs.length) % platformTabs.length;
        const nextTab = platformTabs[nextIndex];
        setPlatform(nextTab.dataset.platformTab, true);
      });
    });
  }

  const navLinks = [...document.querySelectorAll('.primary-nav a[href^="#"]')];
  const sections = navLinks
    .map((link) => document.querySelector(link.getAttribute("href")))
    .filter(Boolean);

  if ("IntersectionObserver" in window && sections.length) {
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) {
          return;
        }

        navLinks.forEach((link) => {
          const isCurrent = link.getAttribute("href") === `#${entry.target.id}`;
          if (isCurrent) {
            link.setAttribute("aria-current", "true");
          } else {
            link.removeAttribute("aria-current");
          }
        });
      });
    }, { rootMargin: "-30% 0px -55%" });

    sections.forEach((section) => observer.observe(section));
  }
})();
