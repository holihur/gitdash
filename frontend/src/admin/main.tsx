import React from "react";
import ReactDOM from "react-dom/client";
import AdminApp from "./App";
import "../index.css";
import { ThemeProvider } from "@/lib/theme";
import { I18nProvider, detectLang, preloadLang } from "@/lib/i18n";

// 先加载当前语言包再渲染，避免管理台首帧闪现英文兜底文案。
void preloadLang(detectLang()).finally(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <ThemeProvider>
        <I18nProvider>
          <AdminApp />
        </I18nProvider>
      </ThemeProvider>
    </React.StrictMode>,
  );
});
