(function () {
  "use strict";

  const App = () => window.go.main.App;

  const el = {
    viewSettings: document.getElementById("view-settings"),
    viewCustom: document.getElementById("view-custom"),
    viewAbout: document.getElementById("view-about"),

    presetList: document.getElementById("presetList"),
    presetMinutes: document.getElementById("presetMinutes"),
    btnAddPreset: document.getElementById("btnAddPreset"),
    chkDisplayOn: document.getElementById("chkDisplayOn"),
    chkAutoStart: document.getElementById("chkAutoStart"),
    chkAutoUpdate: document.getElementById("chkAutoUpdate"),
    trayClickAction: document.getElementById("trayClickAction"),

    tabDuration: document.getElementById("tabDuration"),
    tabUntil: document.getElementById("tabUntil"),
    paneDuration: document.getElementById("paneDuration"),
    paneUntil: document.getElementById("paneUntil"),
    durationTime: document.getElementById("durationTime"),
    btnStartDuration: document.getElementById("btnStartDuration"),
    freeText: document.getElementById("freeText"),
    btnStartFreeText: document.getElementById("btnStartFreeText"),
    untilInput: document.getElementById("untilInput"),
    btnStartUntil: document.getElementById("btnStartUntil"),
    btnCancelCustom: document.getElementById("btnCancelCustom"),

    btnRepo: document.getElementById("btnRepo"),
    btnCloseAbout: document.getElementById("btnCloseAbout"),

    errorLabel: document.getElementById("errorLabel"),
  };

  const views = {
    settings: el.viewSettings,
    custom: el.viewCustom,
    about: el.viewAbout,
  };

  let config = null; // last known ConfigPayload

  function showError(message) {
    el.errorLabel.textContent = message;
    el.errorLabel.hidden = !message;
  }

  function showView(name) {
    if (!views[name]) return;
    Object.entries(views).forEach(([key, node]) => {
      node.hidden = key !== name;
    });
    showError("");
    if (name === "custom") resetCustomForm();
  }

  function formatPresetSec(sec) {
    if (sec <= 0) return "Unlimited";
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (h > 0 && m > 0) return `${h}h ${m}m`;
    if (h > 0) return `${h}h`;
    return `${m}m`;
  }

  function renderPresets() {
    el.presetList.innerHTML = "";
    if (!config.timerList.length) {
      const empty = document.createElement("li");
      empty.className = "preset-empty";
      empty.textContent = "No presets yet — add one below.";
      el.presetList.appendChild(empty);
      return;
    }
    const last = config.timerList.length - 1;
    config.timerList.forEach((sec, index) => {
      const li = document.createElement("li");
      li.className = "preset-item";

      const label = document.createElement("span");
      label.className = "preset-label";
      label.textContent = formatPresetSec(sec);

      const actions = document.createElement("span");
      actions.className = "preset-actions";

      const up = document.createElement("button");
      up.className = "preset-move";
      up.textContent = "▲";
      up.title = "Move up";
      up.disabled = index === 0;
      up.addEventListener("click", () => movePreset(index, -1));

      const down = document.createElement("button");
      down.className = "preset-move";
      down.textContent = "▼";
      down.title = "Move down";
      down.disabled = index === last;
      down.addEventListener("click", () => movePreset(index, 1));

      const remove = document.createElement("button");
      remove.className = "preset-remove";
      remove.textContent = "✕";
      remove.title = "Remove preset";
      remove.addEventListener("click", () => removePreset(index));

      actions.appendChild(up);
      actions.appendChild(down);
      actions.appendChild(remove);

      li.appendChild(label);
      li.appendChild(actions);
      el.presetList.appendChild(li);
    });
  }

  function renderConfig() {
    el.chkDisplayOn.checked = config.keepDisplayOn;
    el.chkAutoStart.checked = config.autoStart;
    el.chkAutoUpdate.checked = config.autoUpdate;
    el.trayClickAction.value = config.trayClickAction;
    renderPresets();
  }

  function setConfig(cfg) {
    config = cfg;
    renderConfig();
  }

  async function saveConfig() {
    try {
      await App().SaveConfig(config);
      showError("");
    } catch (err) {
      showError(String(err));
    }
  }

  function removePreset(index) {
    config.timerList = config.timerList.filter((_, i) => i !== index);
    renderPresets();
    saveConfig();
  }

  function movePreset(index, delta) {
    const target = index + delta;
    if (target < 0 || target >= config.timerList.length) return;
    const list = config.timerList;
    [list[index], list[target]] = [list[target], list[index]];
    renderPresets();
    saveConfig();
  }

  function addPreset() {
    const minutes = parseInt(el.presetMinutes.value, 10);
    if (isNaN(minutes) || minutes < 0) {
      showError("Enter a whole number of minutes (0 = unlimited).");
      return;
    }
    const sec = minutes * 60;
    if (config.timerList.includes(sec)) {
      showError("That preset already exists.");
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
    el.durationTime.value = "00:30:00";
    el.freeText.value = "";
    const local = new Date(Date.now() - new Date().getTimezoneOffset() * 60000);
    local.setSeconds(0, 0);
    el.untilInput.value = local.toISOString().slice(0, 16);
  }

  function setCustomMode(mode) {
    el.tabDuration.classList.toggle("active", mode === "duration");
    el.tabUntil.classList.toggle("active", mode === "until");
    el.paneDuration.hidden = mode !== "duration";
    el.paneUntil.hidden = mode !== "until";
  }

  function parseTimeInputToSeconds(value) {
    // <input type="time" step="1"> gives "HH:MM" or "HH:MM:SS".
    const parts = value.split(":").map((p) => parseInt(p, 10) || 0);
    const [h = 0, m = 0, s = 0] = parts;
    return h * 3600 + m * 60 + s;
  }

  function startAndClose(sec) {
    App()
      .StartDurationSeconds(sec)
      .then(() => App().Hide())
      .catch((err) => showError(String(err)));
  }

  function startDuration() {
    const sec = parseTimeInputToSeconds(el.durationTime.value);
    if (sec <= 0) {
      showError("Enter a duration greater than 0.");
      return;
    }
    startAndClose(sec);
  }

  function startFreeText() {
    const text = el.freeText.value.trim();
    if (!text) return;
    const match = text.match(/(\d+h)?\s*(\d+m)?\s*(\d+s)?/i);
    let sec = 0;
    if (match) {
      if (match[1]) sec += parseInt(match[1], 10) * 3600;
      if (match[2]) sec += parseInt(match[2], 10) * 60;
      if (match[3]) sec += parseInt(match[3], 10);
    }
    if (sec <= 0) {
      showError("Unrecognized format. Try e.g. 1h30m, 45m.");
      return;
    }
    startAndClose(sec);
  }

  function startUntil() {
    if (!el.untilInput.value) {
      showError("Pick a date and time.");
      return;
    }
    const target = new Date(el.untilInput.value);
    const sec = Math.floor((target.getTime() - Date.now()) / 1000);
    if (sec <= 0) {
      showError("Pick a date and time in the future.");
      return;
    }
    startAndClose(sec);
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

    [el.chkDisplayOn, el.chkAutoStart, el.chkAutoUpdate].forEach((chk) => {
      chk.addEventListener("change", () => {
        config.keepDisplayOn = el.chkDisplayOn.checked;
        config.autoStart = el.chkAutoStart.checked;
        config.autoUpdate = el.chkAutoUpdate.checked;
        saveConfig();
      });
    });
    el.trayClickAction.addEventListener("change", () => {
      config.trayClickAction = el.trayClickAction.value;
      saveConfig();
    });

    el.tabDuration.addEventListener("click", () => setCustomMode("duration"));
    el.tabUntil.addEventListener("click", () => setCustomMode("until"));
    el.btnStartDuration.addEventListener("click", startDuration);
    el.btnStartFreeText.addEventListener("click", startFreeText);
    el.freeText.addEventListener("keydown", (e) => {
      if (e.key === "Enter") startFreeText();
    });
    el.btnStartUntil.addEventListener("click", startUntil);
    el.btnCancelCustom.addEventListener("click", () => App().Hide());

    el.btnRepo.addEventListener("click", openRepo);
    el.btnCloseAbout.addEventListener("click", () => App().Hide());

    window.runtime.EventsOn("mugcup:config", setConfig);
    window.runtime.EventsOn("mugcup:view", showView);
  }

  async function init() {
    wireEvents();
    try {
      const [cfg, view] = await Promise.all([App().GetConfig(), App().CurrentView()]);
      setConfig(cfg);
      showView(view);
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
