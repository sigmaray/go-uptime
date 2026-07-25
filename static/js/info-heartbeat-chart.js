(function () {
  'use strict';

  /**
   * Reads a CSS custom property from :root with a fallback.
   * name is the CSS variable including the leading dashes.
   * fallback is used when the variable is missing or empty.
   */
  function cssVar(name, fallback) {
    const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return value || fallback;
  }

  /**
   * Parses the JSON payload embedded next to the chart canvas.
   * root is the [data-info-heartbeat-chart] container element.
   */
  function readChartData(root) {
    const dataEl = root.querySelector('[data-info-heartbeat-chart-data]');
    if (!dataEl) {
      throw new Error('heartbeat chart data element is missing');
    }
    return JSON.parse(dataEl.textContent);
  }

  /**
   * Builds Chart.js stacked bar options for the past-hour heartbeat chart.
   * labels are HH:MM tick labels for each minute.
   * successCounts and failedCounts are aligned per-minute series.
   */
  function createChartConfig(labels, successCounts, failedCounts) {
    const colorUp = cssVar('--info-color-up', '#1f7a54');
    const colorDown = cssVar('--info-color-down', '#b33b3b');
    const colorMuted = cssVar('--info-color-muted', '#6b7280');

    return {
      type: 'bar',
      data: {
        labels: labels,
        datasets: [
          {
            label: 'Successful',
            data: successCounts,
            backgroundColor: colorUp,
            borderSkipped: false,
            borderRadius: 2,
            stack: 'heartbeats',
            barPercentage: 0.9,
            categoryPercentage: 0.95,
          },
          {
            label: 'Failed',
            data: failedCounts,
            backgroundColor: colorDown,
            borderSkipped: false,
            borderRadius: 2,
            stack: 'heartbeats',
            barPercentage: 0.9,
            categoryPercentage: 0.95,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: {
          duration: 400,
        },
        interaction: {
          mode: 'index',
          intersect: false,
        },
        plugins: {
          legend: {
            position: 'bottom',
            labels: {
              boxWidth: 12,
              boxHeight: 12,
              usePointStyle: true,
              pointStyle: 'rectRounded',
              color: colorMuted,
              font: { size: 12 },
            },
          },
          tooltip: {
            callbacks: {
              title: function (items) {
                if (!items.length) {
                  return '';
                }
                return items[0].label;
              },
              footer: function (items) {
                let total = 0;
                items.forEach(function (item) {
                  total += Number(item.raw) || 0;
                });
                return 'Total: ' + total;
              },
            },
          },
        },
        scales: {
          x: {
            stacked: true,
            grid: { display: false },
            ticks: {
              color: colorMuted,
              maxRotation: 0,
              autoSkip: true,
              maxTicksLimit: 8,
              font: { size: 11 },
            },
          },
          y: {
            stacked: true,
            beginAtZero: true,
            grace: '8%',
            ticks: {
              color: colorMuted,
              precision: 0,
              font: { size: 11 },
            },
            grid: {
              color: 'rgba(107, 114, 128, 0.15)',
            },
          },
        },
      },
    };
  }

  /**
   * Initializes one heartbeat hour chart from its data-* hooks.
   * root is a [data-info-heartbeat-chart] element containing canvas + JSON.
   */
  function initHeartbeatChart(root) {
    if (typeof Chart === 'undefined') {
      throw new Error('Chart.js failed to load');
    }

    const canvas = root.querySelector('canvas');
    if (!canvas) {
      throw new Error('heartbeat chart canvas is missing');
    }

    const payload = readChartData(root);
    const labels = Array.isArray(payload.labels) ? payload.labels : [];
    const success = Array.isArray(payload.success) ? payload.success : [];
    const failed = Array.isArray(payload.failed) ? payload.failed : [];

    root.dataset.minuteCount = String(labels.length);

    new Chart(canvas, createChartConfig(labels, success, failed));
  }

  document.querySelectorAll('[data-info-heartbeat-chart]').forEach(function (root) {
    try {
      initHeartbeatChart(root);
    } catch (err) {
      console.error(err);
    }
  });
})();
