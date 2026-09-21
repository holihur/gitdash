import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./index.css";
import { ThemeProvider } from "@/lib/theme";
import { I18nProvider, detectLang, preloadLang } from "@/lib/i18n";
import { loadInstanceInfo } from "@/lib/api";

// 实例信息（SSH 端口、文档地址）不再阻塞首次渲染：先并行发起请求，docsUrl 在
// 到达后通过可订阅 store 通知相关组件补渲染，避免首屏空等一个网络往返。
void loadInstanceInfo();

// 只等待当前语言包（单个小 chunk）即可渲染，避免首帧闪现英文兜底文案；
// 其余语言在切换时才按需加载。
void preloadLang(detectLang()).finally(() => {
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
