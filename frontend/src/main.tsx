import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";
import { ThemeProvider } from "@/lib/theme";
import { I18nProvider } from "@/lib/i18n";
import { loadInstanceInfo } from "@/lib/api";

// 先拉取实例信息（SSH 端口、文档地址）再渲染，保证登录页/克隆地址一次到位。
void loadInstanceInfo().finally(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <ThemeProvider>
        <I18nProvider>
          <App />
        </I18nProvider>
      </ThemeProvider>
    </React.StrictMode>,
  );
});
