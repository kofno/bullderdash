/*
 * app.js — Bull-der-dash shared vanilla-JS helpers.
 *
 * Replaces htmx for the server-rendered pages. Served same-origin via go:embed
 * at /assets/app.js (no CDN). The only live behaviour we need is "periodically
 * re-fetch an HTML fragment endpoint and swap it into a container", which is a
 * thin wrapper over fetch() + innerHTML.
 *
 * Declarative usage (preferred): mark a container with data attributes and the
 * poller wires itself up on DOMContentLoaded.
 *
 *   <div data-poll-url="/queues" data-poll-interval="5000">loading…</div>
 *
 * Attributes:
 *   data-poll-url        (required) fragment endpoint returning HTML
 *   data-poll-interval   (optional) milliseconds between refreshes; default 5000
 *   data-poll-immediate  (optional) "false" to skip the initial fetch
 */
(function () {
  "use strict";

  var DEFAULT_INTERVAL = 5000;

  function swap(el, html) {
    el.innerHTML = html;
  }

  async function fetchFragment(url) {
    var resp = await fetch(url, {
      headers: { Accept: "text/html" },
      credentials: "same-origin",
    });
    if (!resp.ok) {
      throw new Error("fetch " + url + " -> " + resp.status);
    }
    return resp.text();
  }

  // startPoll wires a single container to an endpoint and returns a stop()
  // function. Failures are swallowed (kept on-screen stale) rather than wiping
  // the container, so a transient blip does not blank the dashboard.
  function startPoll(el, options) {
    options = options || {};
    var url = options.url || el.getAttribute("data-poll-url");
    if (!url) return function () {};
    var interval = options.interval ||
      parseInt(el.getAttribute("data-poll-interval"), 10) ||
      DEFAULT_INTERVAL;
    var immediate = options.immediate !== undefined
      ? options.immediate
      : el.getAttribute("data-poll-immediate") !== "false";

    var stopped = false;
    var timer = null;

    async function tick() {
      if (stopped) return;
      try {
        var html = await fetchFragment(url);
        if (!stopped) swap(el, html);
      } catch (err) {
        if (window.console && console.debug) console.debug("[poll]", err);
      } finally {
        if (!stopped) timer = setTimeout(tick, interval);
      }
    }

    if (immediate) {
      tick();
    } else {
      timer = setTimeout(tick, interval);
    }

    // Pause polling while the tab is hidden; resume (and refresh now) on return.
    function onVisibility() {
      if (document.hidden) {
        if (timer) { clearTimeout(timer); timer = null; }
      } else if (!stopped && !timer) {
        tick();
      }
    }
    document.addEventListener("visibilitychange", onVisibility);

    return function stop() {
      stopped = true;
      if (timer) clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }

  function init() {
    var nodes = document.querySelectorAll("[data-poll-url]");
    for (var i = 0; i < nodes.length; i++) {
      startPoll(nodes[i]);
    }
  }

  // Public surface for pages that need to drive polling imperatively.
  window.BDD = window.BDD || {};
  window.BDD.startPoll = startPoll;
  window.BDD.fetchFragment = fetchFragment;

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
