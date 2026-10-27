import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./en";
import vi from "./vi";

export const languages = ["vi", "en"] as const;
export type Language = (typeof languages)[number];

function saved(): Language {
  try {
    const l = localStorage.getItem("customsvn-lang");
    return l === "en" ? "en" : "vi";
  } catch {
    return "vi";
  }
}

void i18n.use(initReactI18next).init({
  resources: { vi: { translation: vi }, en: { translation: en } },
  lng: saved(),
  fallbackLng: "vi",
  interpolation: { escapeValue: false },
});

export function setLanguage(l: Language) {
  void i18n.changeLanguage(l);
  document.documentElement.lang = l;
  try {
    localStorage.setItem("customsvn-lang", l);
  } catch {
    // Private mode: the choice lasts for the session only.
  }
}

export default i18n;
