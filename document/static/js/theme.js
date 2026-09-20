// 与 gitdash 前端共用主题偏好键 gitdash-theme（light | dark | system，默认 system）。
(function () {
  var KEY = "gitdash-theme";
  var mq = window.matchMedia("(prefers-color-scheme: dark)");

  function stored() {
    try {
      var v = localStorage.getItem(KEY);
      if (v === "light" || v === "dark" || v === "system") return v;
    } catch (e) {}
    return "system";
  }

  function resolve(t) {
    return t === "dark" || (t === "system" && mq.matches) ? "dark" : "light";
  }

  function apply() {
    var t = stored();
    document.documentElement.classList.toggle("dark", resolve(t) === "dark");
    return t;
  }

  function syncSelect() {
    var sel = document.getElementById("theme-select");
    if (sel) sel.value = stored();
  }

  // 跟随系统时，响应系统明暗变化。
  function onSystemChange() {
    if (stored() === "system") apply();
  }
  if (mq.addEventListener) mq.addEventListener("change", onSystemChange);
  else if (mq.addListener) mq.addListener(onSystemChange);

  // 多标签页同步。
  window.addEventListener("storage", function (e) {
    if (e.key === KEY) {
      apply();
      syncSelect();
    }
  });

  document.addEventListener("DOMContentLoaded", function () {
    apply();
    syncSelect();
    var sel = document.getElementById("theme-select");
    if (sel) {
      sel.addEventListener("change", function () {
        try {
          localStorage.setItem(KEY, sel.value);
        } catch (e) {}
        apply();
        syncSelect();
      });
    }
  });
})();

// 移动端侧边导航折叠开关。
(function () {
  function toggleNav(force) {
    var open = typeof force === "boolean" ? force : !document.body.classList.contains("nav-open");
    document.body.classList.toggle("nav-open", open);
    var btn = document.querySelector(".nav-toggle");
    if (btn) btn.setAttribute("aria-expanded", open ? "true" : "false");
  }
  document.addEventListener("DOMContentLoaded", function () {
    var btn = document.querySelector(".nav-toggle");
    if (!btn) return;
    btn.addEventListener("click", function () { toggleNav(); });
    var sidebar = document.getElementById("sidebar");
    if (sidebar) {
      sidebar.addEventListener("click", function (e) {
        if (e.target.closest("a")) toggleNav(false);
      });
    }
  });
})();
