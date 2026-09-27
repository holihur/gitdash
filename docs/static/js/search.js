// 站内搜索（Pagefind）。首次打开时再懒加载 pagefind-ui，避免无索引时产生 404。
(function () {
  var base = window.__gitdashPagefind || "/pagefind/";
  var overlay = null;
  var box = null;
  var closeBtn = null;
  var uiReady = false;
  var loading = false;

  // Pagefind UI 文案：按 <html lang> 选取，缺省回退英文。
  var I18N = {
    zh: {
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
    },
    ja: {
      placeholder: "検索",
      clear_search: "クリア",
      load_more: "さらに多くの結果",
      search_label: "サイト内を検索",
      zero_results: "[SEARCH_TERM] の結果はありません",
      many_results: "[COUNT] 件の [SEARCH_TERM] の結果",
      one_result: "[COUNT] 件の [SEARCH_TERM] の結果",
      alt_search: "[SEARCH_TERM] の結果はありません。代わりに [DIFFERENT_TERM] の結果を表示します",
      search_suggestion: "[SEARCH_TERM] の結果はありません。次のような検索を試してください:",
      searching: "[SEARCH_TERM] を検索中…"
    },
    ko: {
      placeholder: "검색",
      clear_search: "지우기",
      load_more: "결과 더 불러오기",
      search_label: "사이트 검색",
      zero_results: "[SEARCH_TERM]에 대한 결과가 없습니다",
      many_results: "[SEARCH_TERM] 관련 결과 [COUNT]개",
      one_result: "[SEARCH_TERM] 관련 결과 [COUNT]개",
      alt_search: "[SEARCH_TERM]에 대한 결과가 없습니다. 대신 [DIFFERENT_TERM]의 결과를 표시합니다",
      search_suggestion: "[SEARCH_TERM]에 대한 결과가 없습니다. 다음 검색을 시도해 보세요:",
      searching: "[SEARCH_TERM] 검색 중…"
    },
    fr: {
      placeholder: "Rechercher",
      clear_search: "Effacer",
      load_more: "Charger plus de résultats",
      search_label: "Rechercher sur le site",
      zero_results: "Aucun résultat pour [SEARCH_TERM]",
      many_results: "[COUNT] résultats pour [SEARCH_TERM]",
      one_result: "[COUNT] résultat pour [SEARCH_TERM]",
      alt_search: "Aucun résultat pour [SEARCH_TERM], affichage à la place des résultats pour [DIFFERENT_TERM]",
      search_suggestion: "Aucun résultat pour [SEARCH_TERM], essayez une recherche parmi :",
      searching: "Recherche de [SEARCH_TERM]…"
    },
    de: {
      placeholder: "Suchen",
      clear_search: "Löschen",
      load_more: "Weitere Ergebnisse laden",
      search_label: "Website durchsuchen",
      zero_results: "Keine Ergebnisse für [SEARCH_TERM]",
      many_results: "[COUNT] Ergebnisse für [SEARCH_TERM]",
      one_result: "[COUNT] Ergebnis für [SEARCH_TERM]",
      alt_search: "Keine Ergebnisse für [SEARCH_TERM], stattdessen Ergebnisse für [DIFFERENT_TERM]",
      search_suggestion: "Keine Ergebnisse für [SEARCH_TERM], versuchen Sie eine Suche nach:",
      searching: "[SEARCH_TERM] wird gesucht…"
    },
    ru: {
      placeholder: "Поиск",
      clear_search: "Очистить",
      load_more: "Загрузить ещё результаты",
      search_label: "Поиск по сайту",
      zero_results: "Ничего не найдено по запросу [SEARCH_TERM]",
      many_results: "[COUNT] результатов по запросу [SEARCH_TERM]",
      one_result: "[COUNT] результат по запросу [SEARCH_TERM]",
      alt_search: "Ничего не найдено по запросу [SEARCH_TERM], показаны результаты по запросу [DIFFERENT_TERM]",
      search_suggestion: "Ничего не найдено по запросу [SEARCH_TERM], попробуйте изменить запрос:",
      searching: "Идёт поиск по запросу [SEARCH_TERM]…"
    },
    es: {
      placeholder: "Buscar",
      clear_search: "Borrar",
      load_more: "Cargar más resultados",
      search_label: "Buscar en el sitio",
      zero_results: "Sin resultados para [SEARCH_TERM]",
      many_results: "[COUNT] resultados para [SEARCH_TERM]",
      one_result: "[COUNT] resultado para [SEARCH_TERM]",
      alt_search: "Sin resultados para [SEARCH_TERM]; se muestran los resultados de [DIFFERENT_TERM]",
      search_suggestion: "Sin resultados para [SEARCH_TERM]; prueba a buscar:",
      searching: "Buscando [SEARCH_TERM]…"
    },
    pt: {
      placeholder: "Pesquisar",
      clear_search: "Limpar",
      load_more: "Carregar mais resultados",
      search_label: "Pesquisar no site",
      zero_results: "Nenhum resultado para [SEARCH_TERM]",
      many_results: "[COUNT] resultados para [SEARCH_TERM]",
      one_result: "[COUNT] resultado para [SEARCH_TERM]",
      alt_search: "Nenhum resultado para [SEARCH_TERM]; exibindo os resultados de [DIFFERENT_TERM]",
      search_suggestion: "Nenhum resultado para [SEARCH_TERM]; tente pesquisar por:",
      searching: "Pesquisando [SEARCH_TERM]…"
    }
  };

  var ERRORS = {
    init: {
      zh: "搜索初始化失败。",
      ja: "検索を初期化できませんでした。",
      ko: "검색을 초기화할 수 없습니다.",
      fr: "Échec de l'initialisation de la recherche.",
      de: "Die Suche konnte nicht initialisiert werden.",
      ru: "Не удалось инициализировать поиск.",
      es: "No se pudo inicializar la búsqueda.",
      pt: "Não foi possível inicializar a pesquisa."
    },
    missingIndex: {
      zh: "搜索索引尚未生成：请先运行 `hugo` 构建，再执行 `pagefind --site docs/public`。",
      ja: "検索インデックスがまだ生成されていません：まず `hugo` でビルドし、次に `pagefind --site docs/public` を実行してください。",
      ko: "검색 인덱스가 아직 생성되지 않았습니다: 먼저 `hugo` 로 빌드한 뒤 `pagefind --site docs/public` 을 실행하세요.",
      fr: "L'index de recherche n'a pas encore été généré : lancez d'abord une compilation avec `hugo`, puis `pagefind --site docs/public`.",
      de: "Der Suchindex wurde noch nicht erzeugt: Führe zuerst einen Build mit `hugo` aus und danach `pagefind --site docs/public`.",
      ru: "Поисковый индекс ещё не создан: сначала выполните сборку `hugo`, затем `pagefind --site docs/public`.",
      es: "El índice de búsqueda aún no se ha generado: ejecuta primero una compilación con `hugo` y luego `pagefind --site docs/public`.",
      pt: "O índice de pesquisa ainda não foi gerado: execute primeiro uma build com `hugo` e depois `pagefind --site docs/public`."
    }
  };

  function langKey() {
    var lang = (document.documentElement.lang || "en").toLowerCase().split("-")[0];
    return lang === "zh" ? "zh" : I18N[lang] ? lang : "en";
  }

  function msg(set) {
    var key = langKey();
    if (key === "en") return "";
    return (set && set[key]) || set.en || "";
  }

  function translations() {
    var key = langKey();
    if (key === "en") return {};
    return I18N[key];
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
      box.textContent = msg(ERRORS.init);
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
      box.textContent = msg(ERRORS.missingIndex);
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
