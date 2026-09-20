// 站内搜索（Pagefind）。首次打开时再懒加载 pagefind-ui，避免无索引时产生 404。
(function () {
  var base = window.__gitdashPagefind || "/pagefind/";
  var overlay = null;
  var box = null;
  var closeBtn = null;
  var uiReady = false;
  var loading = false;

  function translations() {
    var lang = (document.documentElement.lang || "en").toLowerCase();
    if (lang.indexOf("zh") === 0) {
      return {
        placeholder: "搜索",
        clear_search: "清除",
        load_more: "加载更多结果",
        search_label: "搜索本站",
        zero_results: "没有 [SEARCH_TERM] 的相关结果",
        many_results: "[COUNT] 条 [SEARCH_TERM] 相关结果",
        one_result: "[COUNT] 条 [SEARCH_TERM] 相关结果",
        alt_search: "没有 [SEARCH_TERM] 的相关结果，改为显示 [DIFFERENT_TERM] 的结果",
        search_suggestion: "没有 [SEARCH_TERM] 的相关结果，请尝试以下搜索：",
        searching: "正在搜索 [SEARCH_TERM]..."
      };
    }
    return {};
  }

  function focusInput() {
    setTimeout(function () {
      var input = box && box.querySelector("input");
      if (input) input.focus();
    }, 60);
  }

  function initUI() {
    if (uiReady || typeof window.PagefindUI === "undefined") return;
    try {
      new window.PagefindUI({
        element: "#search",
        showSubResults: true,
        showImages: false,
        translations: translations()
      });
      uiReady = true;
    } catch (e) {
      box.textContent = "搜索初始化失败。";
    }
  }

  function load() {
    if (uiReady || loading) return;
    loading = true;

    var css = document.createElement("link");
    css.rel = "stylesheet";
    css.href = base + "pagefind-ui.css";
    document.head.appendChild(css);

    var s = document.createElement("script");
    s.src = base + "pagefind-ui.js";
    s.onload = function () {
      loading = false;
      initUI();
      focusInput();
    };
    s.onerror = function () {
      loading = false;
      box.textContent = "搜索索引尚未生成：请先运行 `hugo` 构建，再执行 `pagefind --site document/public`。";
    };
    document.head.appendChild(s);
  }

  function open() {
    if (!overlay) return;
    overlay.hidden = false;
    document.body.classList.add("search-open");
    load();
    focusInput();
  }

  function close() {
    if (!overlay) return;
    overlay.hidden = true;
    document.body.classList.remove("search-open");
  }

  document.addEventListener("DOMContentLoaded", function () {
    overlay = document.getElementById("search-overlay");
    box = document.getElementById("search");
    closeBtn = document.querySelector(".search-close");
    var btn = document.querySelector(".search-toggle");
    if (!overlay || !box || !btn) return;

    btn.addEventListener("click", open);
    if (closeBtn) closeBtn.addEventListener("click", close);
    overlay.addEventListener("click", function (e) {
      if (e.target === overlay) close();
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && !overlay.hidden) close();
    });
  });
})();
