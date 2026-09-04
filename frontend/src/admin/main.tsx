import React from "react";
import ReactDOM from "react-dom/client";
import AdminApp from "./App";
import "../index.css";
import { ThemeProvider } from "@/lib/theme";
import { I18nProvider } from "@/lib/i18n";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ThemeProvider>
      <I18nProvider>
        <AdminApp />
      </I18nProvider>
    </ThemeProvider>
  </React.StrictMode>,
);
