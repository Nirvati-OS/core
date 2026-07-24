// Minimal, dependency-free i18n: flat dot-path keys, "{name}"-style interpolation.
// To add a language: drop static/i18n/<lang>.json (same keys as en.json) and add it to
// SUPPORTED_LANGUAGES below.

const SUPPORTED_LANGUAGES = ["en", "de"];
const FALLBACK_LANGUAGE = "en";
const STORAGE_KEY = "nirvati-init-lang";

let catalog = {};
let fallbackCatalog = {};
let currentLanguage = FALLBACK_LANGUAGE;

function detectLanguage() {
  const stored = localStorage.getItem(STORAGE_KEY);
  if (stored && SUPPORTED_LANGUAGES.includes(stored)) {
    return stored;
  }

  for (const tag of navigator.languages || [navigator.language]) {
    const short = (tag || "").slice(0, 2).toLowerCase();
    if (SUPPORTED_LANGUAGES.includes(short)) {
      return short;
    }
  }

  return FALLBACK_LANGUAGE;
}

async function loadCatalog(lang) {
  const res = await fetch(`/i18n/${lang}.json`);
  if (!res.ok) {
    throw new Error(`failed to load i18n catalog for ${lang}`);
  }
  return res.json();
}

export async function initI18n(lang) {
  currentLanguage = lang || detectLanguage();

  fallbackCatalog = await loadCatalog(FALLBACK_LANGUAGE);
  catalog = currentLanguage === FALLBACK_LANGUAGE ? fallbackCatalog : await loadCatalog(currentLanguage);

  document.documentElement.lang = currentLanguage;
}

export function setLanguage(lang) {
  localStorage.setItem(STORAGE_KEY, lang);
  return initI18n(lang);
}

export function getLanguage() {
  return currentLanguage;
}

export function supportedLanguages() {
  return SUPPORTED_LANGUAGES;
}

export function t(key, params) {
  let str = catalog[key] ?? fallbackCatalog[key] ?? key;

  if (params) {
    for (const [name, value] of Object.entries(params)) {
      str = str.replaceAll(`{${name}}`, String(value));
    }
  }

  return str;
}

// Applies translations to every element with a data-i18n attribute, using textContent by
// default or the attribute named in data-i18n-attr (e.g. data-i18n-attr="placeholder").
export function applyTranslations(root = document) {
  for (const el of root.querySelectorAll("[data-i18n]")) {
    const key = el.getAttribute("data-i18n");
    const attr = el.getAttribute("data-i18n-attr");

    if (attr) {
      el.setAttribute(attr, t(key));
    } else {
      el.textContent = t(key);
    }
  }
}
