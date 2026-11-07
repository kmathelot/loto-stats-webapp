// Small progressive enhancements on top of htmx. No framework, no build step.
(function () {
  "use strict";

  // ---- Theme: auto (OS) → light → dark, persisted in localStorage ----------
  function currentTheme() {
    var t = document.documentElement.dataset.theme;
    if (t) return t;
    return window.matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark";
  }
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-theme-toggle]");
    if (!btn) return;
    var next = currentTheme() === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem("theme", next); } catch (err) {}
  });

  // ---- Dialogs ---------------------------------------------------------------
  document.addEventListener("click", function (e) {
    var open = e.target.closest("[data-open-dialog]");
    if (open) {
      var d = document.getElementById(open.dataset.openDialog);
      if (d) d.showModal();
      return;
    }
    var close = e.target.closest("[data-close-dialog]");
    if (close) close.closest("dialog").close();
  });

  // ---- Copy a grid to the clipboard -----------------------------------------
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-copy]");
    if (!btn || !navigator.clipboard) return;
    navigator.clipboard.writeText(btn.dataset.copy).then(function () {
      btn.classList.add("copied");
      setTimeout(function () { btn.classList.remove("copied"); }, 1200);
    });
  });

  // ---- Stepper (number of grids) --------------------------------------------
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-step]");
    if (!btn) return;
    var input = btn.closest("[data-stepper]").querySelector("input");
    var v = Math.min(+input.max, Math.max(+input.min, (+input.value || 1) + (+btn.dataset.step)));
    if (v === +input.value) return;
    input.value = v;
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });

  // ---- Selection limits on the verify picker ---------------------------------
  // data-limit="1": behaves like a radio that can be unselected.
  // data-limit="n": once n boxes are checked, the others are disabled.
  function applyLimit(group) {
    var limit = +group.dataset.limit;
    var boxes = group.querySelectorAll("input[type=checkbox]");
    var checked = group.querySelectorAll("input:checked").length;
    boxes.forEach(function (b) { b.disabled = limit > 1 && !b.checked && checked >= limit; });
  }
  document.addEventListener("change", function (e) {
    var group = e.target.closest("[data-limit]");
    if (!group) return;
    if (+group.dataset.limit === 1 && e.target.checked) {
      group.querySelectorAll("input:checked").forEach(function (b) { if (b !== e.target) b.checked = false; });
    }
    applyLimit(group);
  }, true); // capture: run before htmx serializes the form
  function initLimits() { document.querySelectorAll("[data-limit]").forEach(applyLimit); }
  document.addEventListener("DOMContentLoaded", initLimits);
  document.addEventListener("htmx:afterSettle", initLimits);
})();
