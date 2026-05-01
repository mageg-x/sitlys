import { createApp } from "vue";
import { createI18n } from "vue-i18n";
import App from "./App.vue";
import { messages } from "./messages";
import "./styles.css";

const locale = navigator.language.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US";

const i18n = createI18n({
  legacy: false,
  locale,
  fallbackLocale: "en-US",
  messages,
});

createApp(App).use(i18n).mount("#app");
