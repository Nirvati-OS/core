import { initI18n, setLanguage, getLanguage, supportedLanguages, t, applyTranslations } from "./i18n.js";

const screens = {};
let currentScreen = "welcome";

function showScreen(name) {
  for (const el of Object.values(screens)) {
    el.hidden = true;
  }
  screens[name].hidden = false;
  currentScreen = name;
}

function showError(el, message) {
  el.textContent = message;
  el.hidden = !message;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Marker present in this page's own <head> - see index.html. Once "/" answers with something
// that no longer contains it, the Init UI has been replaced by whatever runs next.
const INIT_UI_MARKER = 'name="nirvati-app" content="init"';
const HANDOVER_POLL_INTERVAL_MS = 3000;

// After setup is applied, the server installs and reboots on its own; there's nothing more for
// this page to do except wait for something else to start answering on "/" and then jump to it
// automatically, so the person doesn't have to remember to come back and reload by hand.
async function pollForHandover() {
  for (;;) {
    await sleep(HANDOVER_POLL_INTERVAL_MS);

    let res;
    try {
      res = await fetch("/", { cache: "no-store" });
    } catch {
      // expected for most of this loop: disconnected during install/reboot, or not listening yet
      continue;
    }

    const reachable = res.ok || (res.status >= 200 && res.status < 400);
    if (!reachable) {
      continue;
    }

    const text = await res.text().catch(() => "");
    if (!text.includes(INIT_UI_MARKER)) {
      location.reload();
      return;
    }
  }
}

async function fetchJSON(url, opts) {
  const res = await fetch(url, opts);
  let body = null;
  try {
    body = await res.json();
  } catch {
    // no/invalid JSON body
  }

  if (!res.ok) {
    throw new Error((body && body.error) || `${res.status} ${res.statusText}`);
  }

  return body;
}

async function populateSystemInfo() {
  try {
    const info = await fetchJSON("/api/v1/system-info");
    const endpointInput = document.getElementById("new-endpoint");
    if (info.candidateEndpoints && info.candidateEndpoints.length > 0 && !endpointInput.value) {
      endpointInput.value = info.candidateEndpoints[0];
    }
  } catch (err) {
    // non-fatal: user can still type an address in manually
    console.warn("failed to load system info", err); // eslint-disable-line no-console
  }
}

async function pollDisks() {
  const select = document.getElementById("new-disk");
  const submitBtn = document.getElementById("new-submit");

  const attempt = async () => {
    let disks = [];
    try {
      disks = await fetchJSON("/api/v1/disks");
    } catch (err) {
      console.warn("failed to list disks", err); // eslint-disable-line no-console
    }

    if (disks.length === 0) {
      select.innerHTML = `<option value="">${t("new.disk.loading")}</option>`;
      submitBtn.disabled = true;
      setTimeout(attempt, 1500);
      return;
    }

    select.innerHTML = "";
    for (const disk of disks) {
      const opt = document.createElement("option");
      opt.value = disk.devPath;
      // Lead with size/model - what a beginner recognizes - and tuck the device path in
      // parens as a small disambiguating detail, not the headline.
      const primary = [disk.prettySize, disk.model].filter(Boolean).join(" ") || disk.devPath;
      opt.textContent = primary === disk.devPath ? primary : `${primary} (${disk.devPath})`;
      select.appendChild(opt);
    }
    submitBtn.disabled = false;
  };

  select.innerHTML = `<option value="">${t("new.disk.loading")}</option>`;
  await attempt();
}

function wireWelcome() {
  document.getElementById("choose-new").addEventListener("click", () => {
    showScreen("new");
    populateSystemInfo();
    pollDisks();
  });
  document.getElementById("choose-join").addEventListener("click", () => {
    showScreen("join");
  });
}

function wireBackButtons() {
  for (const btn of document.querySelectorAll("[data-back]")) {
    btn.addEventListener("click", () => showScreen("welcome"));
  }
}

function wireLanguageSwitcher() {
  const select = document.getElementById("language-select");
  for (const lang of supportedLanguages()) {
    const opt = document.createElement("option");
    opt.value = lang;
    opt.textContent = lang.toUpperCase();
    select.appendChild(opt);
  }
  select.value = getLanguage();

  select.addEventListener("change", async () => {
    await setLanguage(select.value);
    applyTranslations();
  });
}

function wireNewClusterForm() {
  const form = document.getElementById("new-form");
  const errorEl = document.getElementById("new-error");
  const submitBtn = document.getElementById("new-submit");

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    showError(errorEl, "");

    // Cluster name always has a sensible default ("nirvati") - a beginner should never be
    // blocked on it, even if they cleared the field while poking around in Advanced.
    const clusterName = document.getElementById("new-cluster-name").value.trim() || "nirvati";
    const installDisk = document.getElementById("new-disk").value;
    const endpointIP = document.getElementById("new-endpoint").value.trim();

    if (!installDisk) {
      showError(errorEl, t("error.required"));
      return;
    }

    if (!endpointIP) {
      // Rare: automatic network-address detection failed. Point them at the one field that
      // actually needs manual attention, rather than a generic error.
      document.querySelector("#new .advanced").open = true;
      showError(errorEl, t("new.endpoint.missing"));
      return;
    }

    submitBtn.disabled = true;
    submitBtn.textContent = t("new.submitting");

    try {
      await fetchJSON("/api/v1/cluster/new", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ clusterName, installDisk, endpointIP }),
      });
      showScreen("done");
      pollForHandover();
    } catch (err) {
      showError(errorEl, err.message || t("error.generic"));
      submitBtn.disabled = false;
      submitBtn.textContent = t("new.submit");
    }
  });
}

function wireJoinForm() {
  const form = document.getElementById("join-form");
  const errorEl = document.getElementById("join-error");
  const submitBtn = document.getElementById("join-submit");
  const textarea = document.getElementById("join-yaml");
  const fileInput = document.getElementById("join-file");

  fileInput.addEventListener("change", async () => {
    const file = fileInput.files[0];
    if (file) {
      textarea.value = await file.text();
    }
  });

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    showError(errorEl, "");

    const yaml = textarea.value.trim();
    if (!yaml) {
      showError(errorEl, t("error.required"));
      return;
    }

    submitBtn.disabled = true;
    submitBtn.textContent = t("join.submitting");

    try {
      await fetchJSON("/api/v1/cluster/join", {
        method: "POST",
        headers: { "Content-Type": "application/yaml" },
        body: yaml,
      });
      showScreen("done");
      pollForHandover();
    } catch (err) {
      showError(errorEl, err.message || t("error.generic"));
      submitBtn.disabled = false;
      submitBtn.textContent = t("join.submit");
    }
  });
}

async function main() {
  for (const el of document.querySelectorAll("[data-screen]")) {
    screens[el.getAttribute("data-screen")] = el;
  }

  try {
    await initI18n();
    applyTranslations();
  } catch (err) {
    // Non-fatal: fall back to the default text already in index.html rather than leaving
    // the whole UI unwired (and therefore unresponsive to clicks) over a translation glitch.
    console.warn("failed to load translations, falling back to default text", err); // eslint-disable-line no-console
  }

  wireLanguageSwitcher();
  wireWelcome();
  wireBackButtons();
  wireNewClusterForm();
  wireJoinForm();

  showScreen(currentScreen);
}

main();
