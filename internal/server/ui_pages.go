package server

const uiPagesJS = `function apiJSON(path, opts = {}) {
  return fetch(path, { headers: { 'Content-Type': 'application/json' }, ...opts })
    .then(async (res) => {
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || ('HTTP ' + res.status));
      }
      return res.json();
    });
}

function buildJobExecutionRow(job, opts = {}) {
  const includeActions = !!opts.includeActions;
  const backPath = opts.backPath || (window.location.pathname || '/');
  const onRemove = opts.onRemove || null;
  const linkClass = opts.linkClass || '';

  const tr = document.createElement('tr');
  const pipeline = (job.metadata && job.metadata.pipeline_id) || '';
  const description = jobDescription(job);
  const backTo = encodeURIComponent(backPath);

  tr.innerHTML =
    '<td><a class="' + linkClass + '" href="/jobs/' + encodeURIComponent(job.id) + '?back=' + backTo + '">' + escapeHtml(description) + '</a></td>' +
    '<td class="' + statusClass(job.status) + '">' + escapeHtml(formatJobStatus(job)) + '</td>' +
    '<td>' + escapeHtml(pipeline) + '</td>' +
    '<td>' + escapeHtml(job.leased_by_agent_id || '') + '</td>' +
    '<td>' + escapeHtml(formatTimestamp(job.created_utc)) + '</td>';

  if (includeActions) {
    const actionTd = document.createElement('td');
    if (['queued', 'leased'].includes((job.status || '').toLowerCase())) {
      const btn = document.createElement('button');
      btn.className = 'secondary';
      btn.textContent = 'Remove';
      btn.onclick = async () => {
        btn.disabled = true;
        try {
          if (onRemove) {
            await onRemove(job);
          }
        } finally {
          btn.disabled = false;
        }
      };
      actionTd.appendChild(btn);
    }
    tr.appendChild(actionTd);
  }

  return tr;
}
`
