// WMS Architecture Portal — shared behavior: theme toggle + sidebar active-link + simple search filter.
(function () {
  var KEY = "wms-arch-theme";

  function applyTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    try { localStorage.setItem(KEY, theme); } catch (e) {}
  }

  function initTheme() {
    var saved = null;
    try { saved = localStorage.getItem(KEY); } catch (e) {}
    if (saved === "light" || saved === "dark") applyTheme(saved);
  }

  function wireToggle() {
    var btn = document.querySelector("[data-theme-toggle]");
    if (!btn) return;
    btn.addEventListener("click", function () {
      var current = document.documentElement.getAttribute("data-theme");
      var isDark = current === "dark" || (!current && window.matchMedia("(prefers-color-scheme: dark)").matches);
      applyTheme(isDark ? "light" : "dark");
    });
  }

  function markActiveLink() {
    var links = document.querySelectorAll(".sidebar nav a[href]");
    var here = window.location.pathname.split("/").pop() || "index.html";
    links.forEach(function (a) {
      var href = a.getAttribute("href");
      var file = href.split("/").pop();
      if (file === here) a.classList.add("active");
    });
  }

  function wireSearch() {
    var input = document.querySelector("[data-search]");
    if (!input) return;
    var targetSelector = input.getAttribute("data-search");
    var items = document.querySelectorAll(targetSelector);
    input.addEventListener("input", function () {
      var q = input.value.trim().toLowerCase();
      items.forEach(function (el) {
        var text = el.textContent.toLowerCase();
        el.style.display = text.indexOf(q) !== -1 ? "" : "none";
      });
    });
  }

  initTheme();
  document.addEventListener("DOMContentLoaded", function () {
    wireToggle();
    markActiveLink();
    wireSearch();
  });
})();
