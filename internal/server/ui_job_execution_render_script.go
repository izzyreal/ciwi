package server

const jobExecutionRenderJS = `
    function formatJobDetailUnmetRequirementHTML(reason) {
      const text = String(reason || '').trim();
      if (!text) return '';
      let m = text.match(/^missing tool\s+(.+)$/i);
      if (m) return 'Missing tool <code>' + escapeHtml(String(m[1] || '').trim()) + '</code>';
      m = text.match(/^tool\s+(\S+)\s+unavailable$/i);
      if (m) return 'Tool <code>' + escapeHtml(String(m[1] || '').trim()) + '</code> unavailable';
      m = text.match(/^tool\s+(\S+)\s+does not satisfy\s+(.+)$/i);
      if (m) return 'Tool <code>' + escapeHtml(String(m[1] || '').trim()) + '</code> does not satisfy <code>' + escapeHtml(String(m[2] || '').trim()) + '</code>';
      return escapeHtml(text);
    }

    function renderModeValue(dryRun) {
      const label = dryRun ? 'Dry run' : 'Ordinary run';
      return '' +
        '<span class="mode-value">' +
          '<span>' + label + '</span>' +
          '<span class="mode-info" tabindex="0" aria-label="Run mode info" data-mode="' + (dryRun ? 'dry' : 'ordinary') + '">' +
            '<span aria-hidden="true">ⓘ</span>' +
          '</span>' +
        '</span>';
    }

    function renderCacheStats(stats) {
      const box = document.getElementById('cacheStatsBox');
      if (!box) return;
      const entries = Array.isArray(stats) ? stats : [];
      if (!entries.length) {
        box.className = 'cache-stats-empty';
        box.textContent = 'No cache statistics reported for this job.';
        return;
      }
      box.className = 'cache-stats-list';
      box.innerHTML = entries.map(s => {
        const id = String((s && s.id) || '').trim();
        const env = String((s && s.env) || '').trim();
        const typ = String((s && s.type) || '').trim();
        const source = String((s && s.source) || '').trim();
        const path = String((s && s.path) || '').trim();
        const files = Number((s && s.files) || 0);
        const dirs = Number((s && s.directories) || 0);
        const size = Number((s && s.size_bytes) || 0);
        const err = String((s && s.error) || '').trim();
        const metrics = (s && s.tool_metrics) || {};
        const metricRows = Object.keys(metrics).sort((a,b) => a.localeCompare(b)).slice(0, 10).map(k =>
          '<div><code>' + escapeHtml(k) + '</code>: ' + escapeHtml(String(metrics[k] || '')) + '</div>'
        ).join('');
        return '' +
          '<div class="cache-stat-item">' +
            '<div class="cache-stat-head">' +
              '<span class="cache-stat-title">' + escapeHtml(id || 'cache') + '</span>' +
              (env ? ('<span class="cache-stat-pill">' + escapeHtml(env) + '</span>') : '') +
              (typ ? ('<span class="cache-stat-pill">' + escapeHtml(typ) + '</span>') : '') +
              (source ? ('<span class="cache-stat-pill">source: ' + escapeHtml(source) + '</span>') : '') +
            '</div>' +
            (path ? ('<div class="cache-stat-row">Path: <code>' + escapeHtml(path) + '</code></div>') : '') +
            '<div class="cache-stat-row">Size: ' + escapeHtml(formatBytes(size)) + ' | Files: ' + escapeHtml(String(files)) + ' | Dirs: ' + escapeHtml(String(dirs)) + '</div>' +
            (err ? ('<div class="cache-stat-row" style="color:#b23a48;">Error: ' + escapeHtml(err) + '</div>') : '') +
            (metricRows ? ('<div class="cache-stat-metrics">' + metricRows + '</div>') : '') +
          '</div>';
      }).join('');
    }

    function normalizeVersionLike(v) {
      const raw = String(v || '').trim();
      if (!raw) return '';
      let out = raw.replace(/^go/i, '').replace(/^v/i, '');
      return out.trim();
    }

    function compareVersionLike(a, b) {
      const pa = normalizeVersionLike(a).split('.').map(s => Number.parseInt(s, 10)).filter(n => Number.isFinite(n));
      const pb = normalizeVersionLike(b).split('.').map(s => Number.parseInt(s, 10)).filter(n => Number.isFinite(n));
      if (!pa.length || !pb.length) return null;
      const n = Math.max(pa.length, pb.length);
      for (let i = 0; i < n; i += 1) {
        const av = i < pa.length ? pa[i] : 0;
        const bv = i < pb.length ? pb[i] : 0;
        if (av < bv) return -1;
        if (av > bv) return 1;
      }
      return 0;
    }

    function toolConstraintSatisfied(actual, constraint) {
      const av = String(actual || '').trim();
      const c = String(constraint || '').trim();
      if (!av) return false;
      if (!c || c === '*') return true;
      let op = '';
      let target = c;
      ['>=', '<=', '>', '<', '==', '='].forEach(candidate => {
        if (!op && c.startsWith(candidate)) {
          op = candidate;
          target = c.slice(candidate.length).trim();
        }
      });
      if (!target) return true;
      if (!op) return av === target;
      const cmp = compareVersionLike(av, target);
      if (cmp == null) {
        return (op === '=' || op === '==') ? av === target : false;
      }
      if (op === '>') return cmp > 0;
      if (op === '>=') return cmp >= 0;
      if (op === '<') return cmp < 0;
      if (op === '<=') return cmp <= 0;
      return cmp === 0;
    }

    function requirementRows(requiredCaps, prefix) {
      const out = [];
      Object.keys(requiredCaps || {}).forEach(k => {
        if (!k.startsWith(prefix)) return;
        const tool = k.slice(prefix.length).trim();
        if (!tool) return;
        out.push({ tool: tool, constraint: String(requiredCaps[k] || '').trim() });
      });
      out.sort((a, b) => a.tool.localeCompare(b.tool));
      return out;
    }

    function renderToolRequirements(requiredCaps, runtimeCaps, jobStatus, unmetRequirements) {
      const req = requiredCaps || {};
      const caps = runtimeCaps || {};
      const unmet = Array.isArray(unmetRequirements) ? unmetRequirements : [];
      const hostRows = requirementRows(req, 'requires.tool.');
      const containerRows = requirementRows(req, 'requires.container.tool.');
      const status = String(jobStatus || '').trim().toLowerCase();

      function renderInto(boxId, rows, prefix, emptyText) {
        const box = document.getElementById(boxId);
        if (!box) return;
        if (!rows.length) {
          box.className = 'req-empty';
          box.textContent = emptyText;
          return;
        }
        const hasObservedRuntimeData = rows.some(r => {
          const key = prefix + r.tool;
          return String(caps[key] || '').trim() !== '';
        });
        if (!hasObservedRuntimeData) {
          if (prefix === 'host.tool.' && unmet.length > 0) {
            const queuedIssues = unmet.map(formatJobDetailUnmetRequirementHTML).filter(Boolean);
            box.className = 'req-issues';
            box.innerHTML = '<strong>Requirements mismatch</strong><ul>' + queuedIssues.map(i => '<li>' + i + '</li>').join('') + '</ul>';
            return;
          }
          box.className = 'req-empty';
          if (isQueuedJobStatus(status) || isRunningJobStatus(status)) {
            box.textContent = 'Pending runtime capability report from agent.';
          } else {
            box.textContent = 'Runtime capability report unavailable for this execution.';
          }
          return;
        }
        const issues = [];
        rows.forEach(r => {
          const actual = String(caps[prefix + r.tool] || '').trim();
          if (!toolConstraintSatisfied(actual, r.constraint)) {
            issues.push('<code>' + escapeHtml(r.tool) + '</code> expected <code>' + escapeHtml(r.constraint || '*') + '</code>, got <code>' + escapeHtml(actual || 'missing') + '</code>');
          }
        });
        if (!issues.length) {
          box.className = 'req-ok';
          box.innerHTML = '<strong>Requirements matched</strong>';
          return;
        }
        box.className = 'req-issues';
        box.innerHTML = '<strong>Requirements mismatch</strong><ul>' + issues.map(i => '<li>' + i + '</li>').join('') + '</ul>';
      }

      renderInto('hostToolReqBox', hostRows, 'host.tool.', 'No tool requirements declared for this job.');
      renderInto('containerToolReqBox', containerRows, 'container.tool.', 'No container tool requirements declared for this job.');
    }
    function buildArtifactTree(items) {
      const root = { dirs: {}, files: [] };
      items.forEach((a) => {
        const raw = String((a && a.path) || '').trim();
        if (!raw) return;
        const parts = raw.split('/').filter(Boolean);
        if (parts.length === 0) return;
        let node = root;
        for (let i = 0; i < parts.length - 1; i += 1) {
          const seg = parts[i];
          if (!node.dirs[seg]) node.dirs[seg] = { dirs: {}, files: [] };
          node = node.dirs[seg];
        }
        node.files.push({ name: parts[parts.length - 1], item: a });
      });
      return root;
    }

    function collectArtifactExpandedPaths(box) {
      const out = new Set();
      if (!box) return out;
      box.querySelectorAll('details[data-artifact-dir]').forEach(d => {
        const p = String(d.getAttribute('data-artifact-dir') || '').trim();
        if (d.open && p) out.add(p);
      });
      return out;
    }

    function renderArtifactTreeNode(node, parentPath, depth, expanded, jobId) {
      const dirNames = Object.keys(node.dirs).sort((a, b) => a.localeCompare(b));
      const files = (node.files || []).slice().sort((a, b) => a.name.localeCompare(b.name));
      let html = '<ul class="artifact-tree">';
      dirNames.forEach(name => {
        const path = parentPath ? (parentPath + '/' + name) : name;
        const open = expanded.has(path);
        const zipHref = '/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts/download?prefix=' + encodeURIComponent(path);
        html += '<li><details data-artifact-dir="' + escapeHtml(path) + '"' + (open ? ' open' : '') + '><summary>' + escapeHtml(name) + ' <a class="artifact-dir-download" href="' + zipHref + '" onclick="event.stopPropagation()">Download .zip</a></summary>' + renderArtifactTreeNode(node.dirs[name], path, depth + 1, expanded, jobId) + '</details></li>';
      });
      files.forEach(entry => {
        const a = entry.item || {};
        html += '' +
          '<li class="artifact-leaf">' +
            '<div class="artifact-row">' +
              '<span class="artifact-path">' + escapeHtml(entry.name) + '</span>' +
              '<span>(' + formatBytes(a.size_bytes) + ')</span>' +
              '<a href=\"' + a.url + '\" target=\"_blank\" rel=\"noopener\">Download</a>' +
            '</div>' +
          '</li>';
      });
      html += '</ul>';
      return html;
    }

    function renderArtifacts(box, jobId, items) {
      const downloadAllBtn = document.getElementById('artifactsDownloadAllBtn');
      const signature = JSON.stringify(items.map(a => [String(a.path || ''), Number(a.size_bytes || 0), String(a.url || '')]));
      if (signature === lastArtifactsSignature) {
        return;
      }
      const previousExpanded = collectArtifactExpandedPaths(box);
      if (previousExpanded.size > 0) {
        artifactExpandedPaths = previousExpanded;
      }
      if (items.length === 0) {
        if (downloadAllBtn) {
          downloadAllBtn.style.display = 'none';
          downloadAllBtn.setAttribute('href', '#');
        }
        box.textContent = 'No artifacts';
        lastArtifactsSignature = signature;
        return;
      }
      if (downloadAllBtn) {
        downloadAllBtn.style.display = '';
        downloadAllBtn.setAttribute('href', '/api/v1/jobs/' + encodeURIComponent(jobId) + '/artifacts/download-all');
      }
      const tree = buildArtifactTree(items);
      const expanded = (artifactExpandedPaths && artifactExpandedPaths.size > 0)
        ? new Set(artifactExpandedPaths)
        : new Set();
      if (expanded.size === 0) {
        // Default expansion is one directory level from root.
        Object.keys(tree.dirs || {}).forEach(name => expanded.add(name));
      }
      box.innerHTML = renderArtifactTreeNode(tree, '', 0, expanded, jobId);
      box.querySelectorAll('details[data-artifact-dir]').forEach(d => {
        d.addEventListener('toggle', () => {
          const path = String(d.getAttribute('data-artifact-dir') || '').trim();
          if (!path) return;
          if (artifactExpandedPaths == null) artifactExpandedPaths = new Set();
          if (d.open) artifactExpandedPaths.add(path);
          else artifactExpandedPaths.delete(path);
        });
      });
      artifactExpandedPaths = collectArtifactExpandedPaths(box);
      lastArtifactsSignature = signature;
    }

    function coverageTotals(c) {
      const total = Number(c.total_statements || c.total_lines || 0);
      const covered = Number(c.covered_statements || c.covered_lines || 0);
      return { total: total, covered: covered };
    }

    function coverageFileTotals(f) {
      const total = Number(f.total_statements || f.total_lines || 0);
      const covered = Number(f.covered_statements || f.covered_lines || 0);
      return { total: total, covered: covered };
    }

    function pct(covered, total) {
      if (!total) return 0;
      return (100 * covered) / total;
    }

    function renderCoverageReport(coverage) {
      const box = document.getElementById('coverageReportBox');
      if (!box) return;
      const openState = {};
      box.querySelectorAll('details[data-cov-key]').forEach(d => {
        const key = String(d.getAttribute('data-cov-key') || '');
        if (key) openState[key] = !!d.open;
      });
      if (!coverage) {
        box.textContent = 'No parsed coverage report';
        return;
      }
      const files = Array.isArray(coverage.files) ? coverage.files.slice() : [];
      const overall = coverageTotals(coverage);
      const overallPct = Number(coverage.percent || pct(overall.covered, overall.total) || 0);

      const modules = new Map();
      files.forEach(f => {
        const path = String(f.path || '').trim();
        const slash = path.lastIndexOf('/');
        const moduleName = slash > 0 ? path.slice(0, slash) : '.';
        const t = coverageFileTotals(f);
        const prev = modules.get(moduleName) || { total: 0, covered: 0, files: 0 };
        prev.total += t.total;
        prev.covered += t.covered;
        prev.files += 1;
        modules.set(moduleName, prev);
      });
      const moduleRows = Array.from(modules.entries())
        .sort((a, b) => pct(a[1].covered, a[1].total) - pct(b[1].covered, b[1].total))
        .map(([name, m]) =>
          '<tr>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;"><code>' + escapeHtml(name) + '</code></td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;">' + m.files + '</td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;">' + m.covered + '/' + m.total + '</td>' +
          '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;"><strong>' + pct(m.covered, m.total).toFixed(2) + '%</strong></td>' +
          '</tr>'
        ).join('');

      const fileRows = files
        .slice()
        .sort((a, b) => pct(coverageFileTotals(a).covered, coverageFileTotals(a).total) - pct(coverageFileTotals(b).covered, coverageFileTotals(b).total))
        .map(f => {
          const t = coverageFileTotals(f);
          return '<tr>' +
            '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;"><code>' + escapeHtml(String(f.path || '')) + '</code></td>' +
            '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;">' + t.covered + '/' + t.total + '</td>' +
            '<td style="padding:4px 6px;border-bottom:1px solid #d7e6dd;text-align:right;"><strong>' + pct(t.covered, t.total).toFixed(2) + '%</strong></td>' +
            '</tr>';
        }).join('');

      const root = { name: '/', children: new Map(), total: 0, covered: 0, isFile: false };
      files.forEach(f => {
        const path = String(f.path || '').trim();
        if (!path) return;
        const t = coverageFileTotals(f);
        const parts = path.split('/').filter(Boolean);
        let node = root;
        node.total += t.total;
        node.covered += t.covered;
        parts.forEach((part, idx) => {
          const key = idx === parts.length - 1 ? 'f:' + part : 'd:' + part;
          if (!node.children.has(key)) {
            node.children.set(key, { name: part, children: new Map(), total: 0, covered: 0, isFile: idx === parts.length - 1 });
          }
          node = node.children.get(key);
          node.total += t.total;
          node.covered += t.covered;
        });
      });

      function nodeHtml(node, prefix) {
        const nodeKey = prefix ? (prefix + '/' + node.name) : node.name;
        const children = Array.from(node.children.values())
          .sort((a, b) => {
            if (a.isFile !== b.isFile) return a.isFile ? 1 : -1;
            return a.name.localeCompare(b.name);
          })
          .map(ch => nodeHtml(ch, nodeKey))
          .join('');
        const label = escapeHtml(node.name) + ' - ' + node.covered + '/' + node.total + ' (' + pct(node.covered, node.total).toFixed(2) + '%)';
        if (!children) {
          return '<li><code>' + label + '</code></li>';
        }
        const isOpen = Object.prototype.hasOwnProperty.call(openState, 'tree:' + nodeKey) ? !!openState['tree:' + nodeKey] : false;
        return '<li><details data-cov-key="tree:' + escapeHtml(nodeKey) + '"' + (isOpen ? ' open' : '') + '><summary><code>' + label + '</code></summary><ul style="margin:6px 0 0 18px;padding:0 0 0 12px;">' + children + '</ul></details></li>';
      }
      const tree = '<ul style="margin:6px 0 0 0;padding:0 0 0 12px;">' + Array.from(root.children.values()).map(ch => nodeHtml(ch, '')).join('') + '</ul>';
      const openModules = Object.prototype.hasOwnProperty.call(openState, 'modules') ? !!openState.modules : true;
      const openFiles = Object.prototype.hasOwnProperty.call(openState, 'files') ? !!openState.files : false;
      const openTree = Object.prototype.hasOwnProperty.call(openState, 'tree') ? !!openState.tree : false;

      box.innerHTML =
        '<div style="margin:0 0 10px;padding:8px;border:1px solid #c4ddd0;border-radius:6px;background:#f6fbf8;">' +
          '<div><strong>Format:</strong> ' + escapeHtml(String(coverage.format || '')) + '</div>' +
          '<div><strong>Overall:</strong> ' + overallPct.toFixed(2) + '% (' + overall.covered + '/' + overall.total + ')</div>' +
          '<div><strong>Files:</strong> ' + files.length + '</div>' +
        '</div>' +
        '<details data-cov-key="modules"' + (openModules ? ' open' : '') + '><summary><strong>By Module</strong></summary>' +
          '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
          '<thead><tr><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Module</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Files</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Covered/Total</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Coverage</th></tr></thead>' +
          '<tbody>' + moduleRows + '</tbody></table>' +
        '</details>' +
        '<details data-cov-key="files"' + (openFiles ? ' open' : '') + '><summary><strong>By File</strong></summary>' +
          '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
          '<thead><tr><th style="text-align:left;border-bottom:1px solid #c4ddd0;">File</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Covered/Total</th><th style="text-align:right;border-bottom:1px solid #c4ddd0;">Coverage</th></tr></thead>' +
          '<tbody>' + fileRows + '</tbody></table>' +
        '</details>' +
        '<details data-cov-key="tree"' + (openTree ? ' open' : '') + '><summary><strong>Tree View</strong></summary>' + tree + '</details>';
    }

    function parseRepoContext(repoURL) {
      const raw = String(repoURL || '').trim();
      if (!raw) return { host: '', repoPath: '' };
      let host = '';
      let repoPath = '';
      let m = raw.match(/^https?:\/\/([^/]+)\/(.+)$/i);
      if (m) {
        host = String(m[1] || '').toLowerCase();
        repoPath = String(m[2] || '');
      } else {
        m = raw.match(/^git@([^:]+):(.+)$/i);
        if (m) {
          host = String(m[1] || '').toLowerCase();
          repoPath = String(m[2] || '');
        } else {
          m = raw.match(/^([^/]+\.[^/]+)\/(.+)$/i);
          if (m) {
            host = String(m[1] || '').toLowerCase();
            repoPath = String(m[2] || '');
          }
        }
      }
      repoPath = repoPath.replace(/\.git$/i, '').replace(/^\/+/, '').replace(/\/+$/, '');
      return { host: host, repoPath: repoPath };
    }

    function deriveTestSourceContext(job) {
      const j = job || {};
      const meta = (j.metadata || {});
      const src = (j.source || {});
      const repo = String(meta.pipeline_source_repo || src.repo || '').trim();
      const ref = String(meta.pipeline_source_ref_resolved || src.ref || '').trim();
      const parsed = parseRepoContext(repo);
      return { host: parsed.host, repoPath: parsed.repoPath, ref: ref };
    }

    function testCaseMatchesFilter(c, filter) {
      if (filter === 'all') return true;
      return String((c && c.status) || '').toLowerCase() === filter;
    }

    function testCaseStatusRank(c) {
      const st = String((c && c.status) || '').toLowerCase();
      if (st === 'fail') return 0;
      if (st === 'skip') return 1;
      if (st === 'pass') return 2;
      return 3;
    }

    function normalizeTestPath(path) {
      let p = String(path || '').trim();
      if (!p) return '';
      p = p.replace(/\\/g, '/');
      while (p.indexOf('./') === 0) p = p.slice(2);
      return p;
    }

    function testPackageRelativePath(pkg, sourceCtx) {
      const sc = sourceCtx || {};
      const host = String(sc.host || '').trim().toLowerCase();
      const repoPath = String(sc.repoPath || '').trim();
      const p = normalizeTestPath(pkg);
      if (!p || !repoPath) return '';
      const fullPrefix = (host ? (host + '/') : '') + repoPath + '/';
      if (p.indexOf(fullPrefix) === 0) return p.slice(fullPrefix.length);
      const ghPrefix = 'github.com/' + repoPath + '/';
      if (p.indexOf(ghPrefix) === 0) return p.slice(ghPrefix.length);
      const glPrefix = 'gitlab.com/' + repoPath + '/';
      if (p.indexOf(glPrefix) === 0) return p.slice(glPrefix.length);
      return '';
    }

    function resolveTestCaseSourcePath(testCase, sourceCtx) {
      const c = testCase || {};
      const sc = sourceCtx || {};
      const repoPath = String(sc.repoPath || '').trim();
      const host = String(sc.host || '').trim().toLowerCase();
      const relPkg = testPackageRelativePath(c.package, sourceCtx);
      let file = normalizeTestPath(c.file);
      if (!file) return '';
      const fullPrefix = (host ? (host + '/') : '') + repoPath + '/';
      if (repoPath && file.indexOf(fullPrefix) === 0) file = file.slice(fullPrefix.length);
      if (repoPath && file.indexOf(repoPath + '/') === 0) file = file.slice(repoPath.length + 1);
      if (repoPath && file.indexOf('github.com/' + repoPath + '/') === 0) file = file.slice(('github.com/' + repoPath + '/').length);
      if (repoPath && file.indexOf('gitlab.com/' + repoPath + '/') === 0) file = file.slice(('gitlab.com/' + repoPath + '/').length);
      if (relPkg && file.indexOf('/') < 0) file = relPkg + '/' + file;
      return normalizeTestPath(file);
    }

    function buildBlobURL(sourceCtx, relPath, line) {
      const sc = sourceCtx || {};
      const host = String(sc.host || '').trim().toLowerCase();
      const repoPath = String(sc.repoPath || '').trim();
      const ref = String(sc.ref || '').trim();
      const path = normalizeTestPath(relPath);
      if (!host || !repoPath || !ref || !path) return '';
      if (host === 'github.com') {
        return 'https://github.com/' + repoPath + '/blob/' + encodeURIComponent(ref) + '/' + encodeURI(path) + (line > 0 ? ('#L' + line) : '');
      }
      if (host === 'gitlab.com') {
        return 'https://gitlab.com/' + repoPath + '/-/blob/' + encodeURIComponent(ref) + '/' + encodeURI(path) + (line > 0 ? ('#L' + line) : '');
      }
      return '';
    }

    function buildTestCaseSourceURL(testCase, sourceCtx) {
      if (!sourceCtx || !sourceCtx.repoPath) return '';
      const c = testCase || {};
      const relPath = resolveTestCaseSourcePath(c, sourceCtx);
      const line = Number(c.line || 0);
      const blobURL = buildBlobURL(sourceCtx, relPath, line);
      if (blobURL) return blobURL;

      const name = String(c.name || '').trim();
      if (!name) return '';
      const repoPath = String(sourceCtx.repoPath || '').trim();
      const host = String(sourceCtx.host || '').trim().toLowerCase();
      const ref = String(sourceCtx.ref || '').trim();
      const relPkg = testPackageRelativePath(c.package, sourceCtx);
      const terms = ['"' + name + '"'];
      if (relPkg) terms.push('path:' + relPkg);
      const query = encodeURIComponent(terms.join(' '));
      if (host !== 'github.com') return '';
      let url = 'https://github.com/' + repoPath + '/search?q=' + query + '&type=code';
      if (ref) url += '&ref=' + encodeURIComponent(ref);
      return url;
    }

    function renderTestReport(report, job) {
      const box = document.getElementById('testReportBox');
      if (!box) return;
      const suites = report && Array.isArray(report.suites) ? report.suites : [];
      if (!suites.length) {
        box.textContent = 'No parsed test report';
        return;
      }
      const sourceCtx = deriveTestSourceContext(job);
      if (window.__ciwiTestFilter == null) {
        window.__ciwiTestFilter = (Number(report.failed || 0) > 0) ? 'fail' : 'all';
      }
      const activeFilter = String(window.__ciwiTestFilter || 'all');
      const header = '' +
        '<div class="test-summary-row">' +
          '<span class="test-pill">Total: ' + (report.total || 0) + '</span>' +
          '<span class="test-pill test-pill-pass">Passed: ' + (report.passed || 0) + '</span>' +
          '<span class="test-pill test-pill-fail">Failed: ' + (report.failed || 0) + '</span>' +
          '<span class="test-pill test-pill-skip">Skipped: ' + (report.skipped || 0) + '</span>' +
        '</div>' +
        '<div class="test-filter-row">' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'all' ? ' active' : '') + '" data-test-filter="all">All</button>' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'fail' ? ' active' : '') + '" data-test-filter="fail">Failed</button>' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'skip' ? ' active' : '') + '" data-test-filter="skip">Skipped</button>' +
          '<button type="button" class="test-filter-btn' + (activeFilter === 'pass' ? ' active' : '') + '" data-test-filter="pass">Passed</button>' +
        '</div>';

      const suiteHtml = suites.map((s, suiteIdx) => {
        const cases = Array.isArray(s.cases) ? s.cases : [];
        const modules = new Map();
        cases.forEach(c => {
          const mod = String(c.package || '').trim() || '(root)';
          if (!modules.has(mod)) modules.set(mod, []);
          modules.get(mod).push(c);
        });
        const moduleHtml = Array.from(modules.entries())
          .sort((a, b) => a[0].localeCompare(b[0]))
          .map(([mod, moduleCases], modIdx) => {
            const visibleCases = moduleCases
              .filter(c => testCaseMatchesFilter(c, activeFilter))
              .slice()
              .sort((a, b) => {
                const byStatus = testCaseStatusRank(a) - testCaseStatusRank(b);
                if (byStatus !== 0) return byStatus;
                return String(a.name || '').localeCompare(String(b.name || ''));
              });
            if (!visibleCases.length) return '';
            let mPass = 0;
            let mFail = 0;
            let mSkip = 0;
            visibleCases.forEach(c => {
              const st = String(c.status || '').toLowerCase();
              if (st === 'pass') mPass++;
              else if (st === 'fail') mFail++;
              else if (st === 'skip') mSkip++;
            });
            const rows = visibleCases.map(c => {
              const testName = escapeHtml(c.name || '');
              const sourceURL = buildTestCaseSourceURL(c, sourceCtx);
              const nameCell = sourceURL
                ? ('<a href="' + sourceURL + '" target="_blank" rel="noopener noreferrer">' + testName + '</a>')
                : testName;
              return '<tr>' +
              '<td>' + nameCell + '</td>' +
              '<td>' + escapeHtml(c.status || '') + '</td>' +
              '<td>' + (c.duration_seconds || 0).toFixed(3) + 's</td>' +
              '</tr>';
            }).join('');
            return '<details data-test-key="suite:' + suiteIdx + ':mod:' + modIdx + '">' +
              '<summary><code>' + escapeHtml(mod) + '</code> - total=' + visibleCases.length + ', passed=' + mPass + ', failed=' + mFail + ', skipped=' + mSkip + '</summary>' +
              '<table style="width:100%;border-collapse:collapse;margin-top:6px;font-size:12px;">' +
              '<thead><tr><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Test</th><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Status</th><th style="text-align:left;border-bottom:1px solid #c4ddd0;">Duration</th></tr></thead>' +
              '<tbody>' + rows + '</tbody></table>' +
              '</details>';
          }).filter(Boolean).join('');
        if (!moduleHtml) return '';
        return '<div style="margin-top:10px;">' +
          '<div><strong>' + escapeHtml(s.name || 'suite') + '</strong> (' + escapeHtml(s.format || '') + ')</div>' +
          '<div style="font-size:13px;color:#5f6f67;">total=' + (s.total || 0) + ', passed=' + (s.passed || 0) + ', failed=' + (s.failed || 0) + ', skipped=' + (s.skipped || 0) + '</div>' +
          '<div style="margin-top:6px;display:flex;flex-direction:column;gap:6px;">' + moduleHtml + '</div>' +
          '</div>';
      }).filter(Boolean).join('');
      const emptyMsg = suiteHtml ? '' : '<div style="color:#5f6f67;">No tests for selected filter.</div>';
      box.innerHTML = header + emptyMsg + suiteHtml;
      box.querySelectorAll('[data-test-filter]').forEach(btn => {
        btn.addEventListener('click', () => {
          window.__ciwiTestFilter = String(btn.getAttribute('data-test-filter') || 'all');
          renderTestReport(report, job);
        });
      });
    }
    function classifyLine(rawLine) {
      if (/^\[meta\]/.test(rawLine)) return 'phase-meta';
      if (/^\[checkout\]/.test(rawLine)) return 'phase-checkout';
      if (/^\[run\]/.test(rawLine)) return 'phase-run';
      if (/^[+]{1,2}\s/.test(rawLine)) return 'shell-trace';
      if (/^[+]{1,2}\s*(git push|gh release create|gh release upload)\b/.test(rawLine)) return 'shell-trace risky-cmd';
      return '';
    }

    function highlightTextTokens(rawText) {
      let out = escapeHtml(rawText);
      out = out.replace(/\b(v\d+\.\d+\.\d+)\b/g, '<span class="tok-version">$1</span>');
      out = out.replace(/\b([0-9a-fA-F]{7,40})\b/g, '<span class="tok-sha">$1</span>');
      out = out.replace(/\bduration=([0-9]+(?:\.[0-9]+)?s)\b/g, 'duration=<span class="tok-duration">$1</span>');
      return out;
    }

    function highlightInline(rawLine) {
      const src = String(rawLine || '');
      const urlRE = /https:\/\/[^\s"']+/g;
      let out = '';
      let last = 0;
      let match;
      while ((match = urlRE.exec(src)) !== null) {
        out += highlightTextTokens(src.slice(last, match.index));
        out += '<span class="tok-url">' + escapeHtml(match[0]) + '</span>';
        last = match.index + match[0].length;
      }
      out += highlightTextTokens(src.slice(last));
      return out;
    }

    function renderDryRunSkippedBlock(lines) {
      const cleaned = lines.filter(l => String(l || '').trim() !== '');
      if (!cleaned.length) return '';
      const head = '<div class="log-dryskip-head">[dry-run] skipped step</div>';
      const body = '<div class="log-dryskip-body">' + cleaned.map(highlightInline).join('\n') + '</div>';
      return '<div class="log-dryskip">' + head + body + '</div>';
    }

    function renderDetachedHeadFold(lines) {
      const text = lines.join('\n');
      return '<details class="log-fold"><summary>git detached HEAD advice (collapsed)</summary><pre>' + escapeHtml(text) + '</pre></details>';
    }

    function renderOutputLog(raw) {
      const text = String(raw || '');
      if (!text) return '<span class="log-empty">&lt;no output yet&gt;</span>';
      const lines = text.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n');
      const html = [];
      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (/^\[dry-run\]\s+skipped step:/.test(line)) {
          const skipped = [line.replace(/^\[dry-run\]\s+skipped step:\s*/, '')];
          for (let j = i + 1; j < lines.length; j++) {
            const next = lines[j];
            if (/^\[(meta|checkout|run|dry-run)\]/.test(next) || /^[+]{1,2}\s/.test(next)) {
              i = j - 1;
              break;
            }
            if (j === lines.length - 1) i = j;
            if (next.trim() === '') {
              i = j;
              break;
            }
            skipped.push(next);
          }
          html.push(renderDryRunSkippedBlock(skipped));
          continue;
        }

        if (line.indexOf("You are in 'detached HEAD' state.") === 0) {
          const folded = [line];
          for (let j = i + 1; j < lines.length; j++) {
            const next = lines[j];
            folded.push(next);
            if (next.indexOf("Turn off this advice by setting config variable advice.detachedHead to false") === 0) {
              i = j;
              break;
            }
            if (j === lines.length - 1) i = j;
          }
          html.push(renderDetachedHeadFold(folded));
          continue;
        }

        const cls = classifyLine(line);
        const classAttr = cls ? ' class="log-line ' + cls + '"' : ' class="log-line"';
        html.push('<div' + classAttr + '>' + highlightInline(line) + '</div>');
      }
      return html.join('');
    }
    function renderReleaseSummary(job) {
      const card = document.getElementById('releaseSummaryCard');
      const box = document.getElementById('releaseSummaryBox');
      if (!card || !box) return;

      const m = (job && job.metadata) || {};
      const isReleasePipeline = (m.pipeline_id || '') === 'release';
      if (!isReleasePipeline) {
        card.style.display = 'none';
        box.innerHTML = '';
        return;
      }

      const dryRun = (m.dry_run || '') === '1';
      const versionLabel = String(m.version || m.pipeline_version_raw || '').trim();
      const tagLabel = String(m.tag || m.pipeline_version || '').trim();
      const lines = [];
      lines.push('<div><strong>Mode:</strong> ' + (dryRun ? 'dry-run' : 'live') + '</div>');
      if (versionLabel) lines.push('<div><strong>Version:</strong> ' + escapeHtml(versionLabel) + '</div>');
      if (tagLabel) lines.push('<div><strong>Tag:</strong> ' + escapeHtml(tagLabel) + '</div>');
      if (m.artifacts) lines.push('<div><strong>Assets:</strong> ' + escapeHtml(m.artifacts) + '</div>');
      if (m.next_version) lines.push('<div><strong>Next version:</strong> ' + escapeHtml(m.next_version) + '</div>');
      if (m.auto_bump_branch) lines.push('<div><strong>Auto bump branch:</strong> ' + escapeHtml(m.auto_bump_branch) + '</div>');
      if (lines.length === 1) lines.push('<div class="label">No release metadata reported yet.</div>');

      box.innerHTML = lines.join('');
      card.style.display = '';
    }

`
