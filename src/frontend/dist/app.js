(function () {
  "use strict";

  const App = () => window.go.main.App;

  const el = {
    head: document.getElementById("appHead"),
    viewSettings: document.getElementById("view-settings"),
    viewCustom: document.getElementById("view-custom"),
    viewAbout: document.getElementById("view-about"),

    presetList: document.getElementById("presetList"),
    presetMinutes: document.getElementById("presetMinutes"),
    btnAddPreset: document.getElementById("btnAddPreset"),
    chkDisplayOn: document.getElementById("chkDisplayOn"),
    chkAutoStart: document.getElementById("chkAutoStart"),
    chkAutoUpdateCheck: document.getElementById("chkAutoUpdateCheck"),
    chkAutoUpdateApply: document.getElementById("chkAutoUpdateApply"),
    trayClickAction: document.getElementById("trayClickAction"),
    languageSelect: document.getElementById("languageSelect"),
    btnExportConfig: document.getElementById("btnExportConfig"),
    btnImportConfig: document.getElementById("btnImportConfig"),

    tabDuration: document.getElementById("tabDuration"),
    tabUntil: document.getElementById("tabUntil"),
    paneDuration: document.getElementById("paneDuration"),
    paneUntil: document.getElementById("paneUntil"),
    freeText: document.getElementById("freeText"),
    btnStartDuration: document.getElementById("btnStartDuration"),
    chkWithDate: document.getElementById("chkWithDate"),
    untilTime: document.getElementById("untilTime"),
    untilDateTime: document.getElementById("untilDateTime"),
    btnStartUntil: document.getElementById("btnStartUntil"),
    btnCancelCustom: document.getElementById("btnCancelCustom"),

    aboutVersion: document.getElementById("aboutVersion"),
    btnRepo: document.getElementById("btnRepo"),
    btnCheckUpdate: document.getElementById("btnCheckUpdate"),
    btnCloseAbout: document.getElementById("btnCloseAbout"),
  };

  const views = {
    settings: el.viewSettings,
    custom: el.viewCustom,
    about: el.viewAbout,
  };

  let config = null; // last known ConfigPayload
  let translations = {}; // last known Translations() map, keyed by "ui.*"

  // t looks up key in the active locale, falling back to the key itself —
  // same fallback shape as the Go side's i18n.T, so a key that somehow never
  // got fetched (translations still {} very early) still shows something
  // legible instead of throwing or rendering blank.
  function t(key) {
    return translations[key] || key;
  }

  // applyTranslations re-labels every element tagged data-i18n (textContent),
  // data-i18n-placeholder, or data-i18n-title from map, then resizes since a
  // language swap can change label lengths enough to reflow the window's
  // content height. Called once at init and again on every "mugcup:config"
  // push (setConfig) — cheap enough that re-running it for unrelated config
  // changes isn't worth special-casing.
  function applyTranslations(map) {
    translations = map || {};
    document.querySelectorAll("[data-i18n]").forEach((node) => {
      const key = node.getAttribute("data-i18n");
      if (translations[key] !== undefined) node.textContent = translations[key];
    });
    document.querySelectorAll("[data-i18n-placeholder]").forEach((node) => {
      const key = node.getAttribute("data-i18n-placeholder");
      if (translations[key] !== undefined) node.placeholder = translations[key];
    });
    document.querySelectorAll("[data-i18n-title]").forEach((node) => {
      const key = node.getAttribute("data-i18n-title");
      if (translations[key] !== undefined) node.title = translations[key];
    });
    if (config) renderPresets(); // preset labels/titles are JS-generated, not data-i18n
    scheduleResize();
  }

  function refreshTranslations() {
    App().Translations().then(applyTranslations).catch(() => {});
  }

  // The window has no fixed content height — Go clamps it between the
  // current view's min/maxHeight (see viewSpecs in wailsapp.go) but the
  // exact size is driven from here, since only the DOM knows its own real
  // height. Measures direct children of .app rather than .app itself (which
  // is pinned to 100vh) so it reflects the content's natural size, not the
  // current window size.
  function measureContentHeight() {
    const appEl = document.querySelector(".app");
    const style = getComputedStyle(appEl);
    const gap = parseFloat(style.rowGap || style.gap) || 0;
    const visible = Array.from(appEl.children).filter(
      (c) => !c.hidden && getComputedStyle(c).display !== "none"
    );
    const contentHeight = visible.reduce((sum, c) => sum + c.getBoundingClientRect().height, 0);
    const gaps = gap * Math.max(0, visible.length - 1);
    const paddingY = (parseFloat(style.paddingTop) || 0) + (parseFloat(style.paddingBottom) || 0);
    return Math.ceil(contentHeight + gaps + paddingY);
  }

  function scheduleResize() {
    requestAnimationFrame(() => {
      App().ResizeToContent(measureContentHeight()).catch(() => {});
    });
  }

  // Validation/settings errors surface as a native modal message box rather
  // than an inline banner — the popup is small and the banner sat at the
  // very bottom, easy to miss entirely (confirmed confusing in practice).
  function showError(message) {
    if (!message) return;
    App().ShowError(message).catch(() => {});
  }

  function showView(name) {
    if (!views[name]) return;
    Object.entries(views).forEach(([key, node]) => {
      node.hidden = key !== name;
    });
    // About's own card already shows "mugcup" (with the same emoji as the
    // header) as its title, so the shared header would just be a duplicate.
    el.head.hidden = name === "about";
    if (name === "custom") resetCustomForm();
    scheduleResize();
  }

  function formatPresetSec(sec) {
    if (sec <= 0) return t("ui.common.indefinite");
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (h > 0 && m > 0) return `${h}h ${m}m`;
    if (h > 0) return `${h}h`;
    return `${m}m`;
  }

  // Index of the preset currently being dragged, while a drag is in
  // progress; null otherwise. Reordering happens once on drop rather than
  // live during dragover, so the list doesn't reflow under the cursor.
  let dragIndex = null;

  // Calls scheduleResize() itself since it runs both on the initial render
  // (redundant with showView's own scheduleResize(), harmless) and on every
  // preset add/remove/reorder while the Settings window is already open —
  // the window is dynamic (wailsapp.go's viewSpecs) up to its maxHeight, and
  // scrolls internally (.app's overflow-y: auto) past that.
  function renderPresets() {
    el.presetList.innerHTML = "";
    if (!config.timerList.length) {
      const empty = document.createElement("li");
      empty.className = "preset-empty";
      empty.textContent = t("ui.preset.empty");
      el.presetList.appendChild(empty);
      scheduleResize();
      return;
    }
    config.timerList.forEach((sec, index) => {
      const li = document.createElement("li");
      li.className = "preset-item";
      li.draggable = true;

      const grip = document.createElement("span");
      grip.className = "preset-grip";
      grip.textContent = "⠿";
      grip.setAttribute("aria-hidden", "true");
      grip.title = t("ui.preset.drag_title");

      const label = document.createElement("span");
      label.className = "preset-label";
      label.textContent = formatPresetSec(sec);

      const actions = document.createElement("span");
      actions.className = "preset-actions";

      const remove = document.createElement("button");
      remove.className = "preset-remove";
      remove.textContent = "✕";
      remove.title = t("ui.preset.remove_title");
      remove.addEventListener("click", () => removePreset(index));

      actions.appendChild(remove);

      li.appendChild(grip);
      li.appendChild(label);
      li.appendChild(actions);

      li.addEventListener("dragstart", (e) => {
        dragIndex = index;
        li.classList.add("dragging");
        e.dataTransfer.effectAllowed = "move";
      });
      li.addEventListener("dragend", () => {
        li.classList.remove("dragging");
        dragIndex = null;
      });
      li.addEventListener("dragover", (e) => {
        if (dragIndex === null) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = "move";
        li.classList.toggle("drop-before", index < dragIndex);
        li.classList.toggle("drop-after", index > dragIndex);
      });
      li.addEventListener("dragleave", () => {
        li.classList.remove("drop-before", "drop-after");
      });
      li.addEventListener("drop", (e) => {
        e.preventDefault();
        li.classList.remove("drop-before", "drop-after");
        if (dragIndex === null || dragIndex === index) return;
        movePresetTo(dragIndex, index);
      });

      el.presetList.appendChild(li);
    });
    scheduleResize();
  }

  function renderConfig() {
    el.chkDisplayOn.checked = config.keepDisplayOn;
    el.chkAutoStart.checked = config.autoStart;
    el.chkAutoUpdateCheck.checked = config.autoUpdateCheck;
    el.chkAutoUpdateApply.checked = config.autoUpdateApply;
    // Auto-installing only ever runs behind an auto-check (see
    // maybeAutoCheckUpdate in updateflow.go), so offering it while checking
    // is off is a contradiction — disable it until checking is back on.
    el.chkAutoUpdateApply.disabled = !config.autoUpdateCheck;
    el.trayClickAction.value = config.trayClickAction;
    el.languageSelect.value = config.language;
    renderPresets();
  }

  function setConfig(cfg) {
    config = cfg;
    renderConfig();
    refreshTranslations(); // language may have changed among cfg's fields
  }

  async function saveConfig() {
    try {
      await App().SaveConfig(config);
    } catch (err) {
      showError(String(err));
    }
  }

  function removePreset(index) {
    config.timerList = config.timerList.filter((_, i) => i !== index);
    renderPresets();
    saveConfig();
  }

  function movePresetTo(from, to) {
    const list = config.timerList;
    const [item] = list.splice(from, 1);
    list.splice(to, 0, item);
    renderPresets();
    saveConfig();
  }

  // Parses "1h30m", "45m", "2h", or "1h 2m 3s" into whole seconds — any
  // subset of units works. Anchored, so garbage text doesn't just match a
  // trailing empty prefix. Returns null if nothing recognizable was found.
  function parseDurationText(text) {
    const match = text.trim().match(/^(\d+h)?\s*(\d+m)?\s*(\d+s)?$/i);
    if (!match || (!match[1] && !match[2] && !match[3])) return null;
    let sec = 0;
    if (match[1]) sec += parseInt(match[1], 10) * 3600;
    if (match[2]) sec += parseInt(match[2], 10) * 60;
    if (match[3]) sec += parseInt(match[3], 10);
    return sec;
  }

  // Same as parseDurationText, plus a bare number of minutes (e.g. "90") for
  // backward compatibility with the old plain-minutes preset field.
  function parsePresetDuration(text) {
    text = text.trim();
    if (!text) return null;
    if (/^\d+$/.test(text)) return parseInt(text, 10) * 60;
    return parseDurationText(text);
  }

  function addPreset() {
    const sec = parsePresetDuration(el.presetMinutes.value);
    if (sec === null || sec < 0) {
      showError(t("ui.error.unrecognized_with_indefinite"));
      return;
    }
    if (config.timerList.includes(sec)) {
      showError(t("ui.error.preset_exists"));
      return;
    }
    config.timerList.push(sec);
    el.presetMinutes.value = "";
    renderPresets();
    saveConfig();
  }

  // ---- Custom view ----

  function resetCustomForm() {
    setCustomMode("duration");
    el.freeText.value = "";

    const defaultTarget = new Date(Date.now() + 30 * 60000);
    el.untilTime.value = `${String(defaultTarget.getHours()).padStart(2, "0")}:${String(defaultTarget.getMinutes()).padStart(2, "0")}`;
    const local = new Date(defaultTarget.getTime() - defaultTarget.getTimezoneOffset() * 60000);
    local.setSeconds(0, 0);
    el.untilDateTime.value = local.toISOString().slice(0, 16);

    el.chkWithDate.checked = false;
    el.untilTime.hidden = false;
    el.untilDateTime.hidden = true;
  }

  function setCustomMode(mode) {
    el.tabDuration.classList.toggle("active", mode === "duration");
    el.tabUntil.classList.toggle("active", mode === "until");
    el.paneDuration.hidden = mode !== "duration";
    el.paneUntil.hidden = mode !== "until";
    scheduleResize();
  }

  function toggleWithDate() {
    el.untilTime.hidden = el.chkWithDate.checked;
    el.untilDateTime.hidden = !el.chkWithDate.checked;
    scheduleResize();
  }

  function startAndClose(promise) {
    promise.then(() => App().Hide()).catch((err) => showError(String(err)));
  }

  // "For a duration": free-text only, e.g. "1h30m", "45m".
  function startDuration() {
    const text = el.freeText.value.trim();
    if (!text) {
      showError(t("ui.error.enter_duration"));
      return;
    }
    const sec = parseDurationText(text);
    if (!sec || sec <= 0) {
      showError(t("ui.error.unrecognized_duration"));
      return;
    }
    startAndClose(App().StartDurationSeconds(sec));
  }

  // "Until a date & time": with "With date" off, the target is today at the
  // picked time; with it on, the target is the picked date and time.
  function startUntil() {
    let target;
    if (el.chkWithDate.checked) {
      if (!el.untilDateTime.value) {
        showError(t("ui.error.pick_datetime"));
        return;
      }
      target = new Date(el.untilDateTime.value);
    } else {
      if (!el.untilTime.value) {
        showError(t("ui.error.pick_time"));
        return;
      }
      const [h = 0, m = 0, s = 0] = el.untilTime.value.split(":").map((p) => parseInt(p, 10) || 0);
      target = new Date();
      target.setHours(h, m, s, 0);
    }

    const sec = Math.floor((target.getTime() - Date.now()) / 1000);
    if (sec <= 0) {
      showError(
        el.chkWithDate.checked
          ? t("ui.error.pick_future_datetime")
          : t("ui.error.pick_future_time")
      );
      return;
    }
    startAndClose(App().StartScheduleSeconds(sec));
  }

  // ---- About view ----

  function openRepo() {
    App().OpenRepo();
  }

  function wireEvents() {
    el.btnAddPreset.addEventListener("click", addPreset);
    el.presetMinutes.addEventListener("keydown", (e) => {
      if (e.key === "Enter") addPreset();
    });

    [el.chkDisplayOn, el.chkAutoStart, el.chkAutoUpdateCheck, el.chkAutoUpdateApply].forEach((chk) => {
      chk.addEventListener("change", () => {
        config.keepDisplayOn = el.chkDisplayOn.checked;
        config.autoStart = el.chkAutoStart.checked;
        config.autoUpdateCheck = el.chkAutoUpdateCheck.checked;
        el.chkAutoUpdateApply.disabled = !el.chkAutoUpdateCheck.checked;
        config.autoUpdateApply = el.chkAutoUpdateApply.checked;
        saveConfig();
      });
    });
    el.trayClickAction.addEventListener("change", () => {
      config.trayClickAction = el.trayClickAction.value;
      saveConfig();
    });
    el.languageSelect.addEventListener("change", () => {
      config.language = el.languageSelect.value;
      saveConfig();
    });

    el.btnExportConfig.addEventListener("click", () => {
      App().ExportConfig().catch((err) => showError(String(err)));
    });
    el.btnImportConfig.addEventListener("click", () => {
      App().ImportConfig().catch((err) => showError(String(err)));
    });

    el.tabDuration.addEventListener("click", () => setCustomMode("duration"));
    el.tabUntil.addEventListener("click", () => setCustomMode("until"));
    el.btnStartDuration.addEventListener("click", startDuration);
    el.freeText.addEventListener("keydown", (e) => {
      if (e.key === "Enter") startDuration();
    });
    el.chkWithDate.addEventListener("change", toggleWithDate);
    el.btnStartUntil.addEventListener("click", startUntil);
    [el.untilTime, el.untilDateTime].forEach((input) => {
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter") startUntil();
      });
    });
    el.btnCancelCustom.addEventListener("click", () => App().Hide());

    el.btnRepo.addEventListener("click", openRepo);
    el.btnCheckUpdate.addEventListener("click", () => {
      App().CheckForUpdates().catch((err) => showError(String(err)));
    });
    el.btnCloseAbout.addEventListener("click", () => App().Hide());

    window.runtime.EventsOn("mugcup:config", setConfig);
    window.runtime.EventsOn("mugcup:view", showView);
  }

  async function init() {
    wireEvents();
    try {
      const [cfg, view, ver, variant, trans] = await Promise.all([
        App().GetConfig(),
        App().CurrentView(),
        App().Version(),
        App().BuildVariant(),
        App().Translations(),
      ]);
      // Apply translations before the window ever reveals itself (it stays
      // hidden until showView's scheduleResize measures and centers it —
      // see revealPending in wailsapp.go), so there's no flash of English
      // before the resolved language replaces it.
      applyTranslations(trans);
      setConfig(cfg);
      showView(view);
      el.aboutVersion.textContent = `${t("ui.about.version_prefix")} ${ver} (${variant})`;
    } catch (err) {
      showError(String(err));
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
