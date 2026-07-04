(function () {
  'use strict';

  function showMessage(el, text, isError) {
    if (!el) return;
    el.textContent = text;
    el.classList.toggle('text-danger', Boolean(isError));
    el.classList.toggle('text-success', !isError);
  }

  async function postJSON(url, body) {
    const response = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body || {}),
    });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(data.error || `Request failed (${response.status})`);
    }
    return data;
  }

  function renderSQLResult(container, data) {
    if (!container) return;
    container.innerHTML = '';

    if (data.columns && data.rows) {
      const table = document.createElement('table');
      table.className = 'table table-sm table-bordered dev-tools__result-table';

      const thead = document.createElement('thead');
      const headRow = document.createElement('tr');
      data.columns.forEach((col) => {
        const th = document.createElement('th');
        th.textContent = col;
        headRow.appendChild(th);
      });
      thead.appendChild(headRow);
      table.appendChild(thead);

      const tbody = document.createElement('tbody');
      data.rows.forEach((row) => {
        const tr = document.createElement('tr');
        row.forEach((cell) => {
          const td = document.createElement('td');
          td.textContent = cell;
          tr.appendChild(td);
        });
        tbody.appendChild(tr);
      });
      table.appendChild(tbody);
      container.appendChild(table);
      return;
    }

    const p = document.createElement('p');
    p.className = 'small text-muted mb-0';
    p.textContent = `Rows affected: ${data.rows_affected ?? 0}`;
    container.appendChild(p);
  }

  document.querySelectorAll('[data-dev-tools-clear-table]').forEach((button) => {
    button.addEventListener('click', async () => {
      const select = document.getElementById('clear-table-select');
      const result = document.querySelector('[data-dev-tools-clear-result]');
      const table = select?.value;
      if (!table) return;
      if (!window.confirm(`Clear table "${table}"?`)) return;

      try {
        await postJSON('/admin/tools/clear-table', { table });
        showMessage(result, `Table "${table}" cleared.`, false);
      } catch (err) {
        showMessage(result, err.message, true);
      }
    });
  });

  document.querySelectorAll('[data-dev-tools-seed-monitors]').forEach((button) => {
    button.addEventListener('click', async () => {
      const result = document.querySelector('[data-dev-tools-seed-result]');
      try {
        const data = await postJSON('/admin/tools/seed-monitors', {});
        showMessage(result, `Created ${data.created} monitor URL(s).`, false);
      } catch (err) {
        showMessage(result, err.message, true);
      }
    });
  });

  document.querySelectorAll('[data-dev-tools-test-telegram]').forEach((button) => {
    button.addEventListener('click', async () => {
      const result = document.querySelector('[data-dev-tools-notify-result]');
      try {
        await postJSON('/admin/tools/test-telegram');
        showMessage(result, 'Test Telegram notification sent.', false);
      } catch (err) {
        showMessage(result, err.message, true);
      }
    });
  });

  document.querySelectorAll('[data-dev-tools-test-smtp]').forEach((button) => {
    button.addEventListener('click', async () => {
      const result = document.querySelector('[data-dev-tools-notify-result]');
      try {
        await postJSON('/admin/tools/test-smtp');
        showMessage(result, 'Test email notification sent.', false);
      } catch (err) {
        showMessage(result, err.message, true);
      }
    });
  });

  document.querySelectorAll('[data-dev-tools-test-error]').forEach((button) => {
    button.addEventListener('click', async () => {
      const result = document.querySelector('[data-dev-tools-record-result]');
      try {
        await postJSON('/admin/tools/test-error');
        showMessage(result, 'Test error recorded. Open Errors to view it.', false);
      } catch (err) {
        showMessage(result, err.message, true);
      }
    });
  });

  document.querySelectorAll('[data-dev-tools-test-log]').forEach((button) => {
    button.addEventListener('click', async () => {
      const result = document.querySelector('[data-dev-tools-record-result]');
      try {
        const data = await postJSON('/admin/tools/test-log');
        showMessage(result, `Test event recorded: ${data.message}`, false);
      } catch (err) {
        showMessage(result, err.message, true);
      }
    });
  });

  document.querySelectorAll('[data-dev-tools-execute-sql]').forEach((button) => {
    button.addEventListener('click', async () => {
      const textarea = document.getElementById('sql-query');
      const result = document.querySelector('[data-dev-tools-sql-result]');
      const query = textarea?.value?.trim();
      if (!query) return;

      result.innerHTML = '<p class="small text-muted">Running...</p>';
      try {
        const data = await postJSON('/admin/tools/execute-sql', { query });
        renderSQLResult(result, data);
      } catch (err) {
        result.innerHTML = '';
        showMessage(result, err.message, true);
      }
    });
  });
})();
