(function () {
  'use strict';

  window.ciwiCreateTreeViewRenderer = function ({
    resolve, renderText, semanticTone, bindActions, rendererKeyPart,
    annotateRendererElement, disclosureStates,
  }) {
    function renderTreeView(element, node, data, context) {
	const tree = node.treeView || {};
	const filter = tree.filter ? String(resolve(data, tree.filter) || 'all') : '';
	const source = resolve(data, tree.nodes);

	function prepared(raw, depth) {
	  const itemData = Object.assign({}, data, {[tree.as]: raw});
	  const rawChildren = resolve(itemData, tree.children);
	  const children = preparedList(rawChildren, depth + 1);
	  const filterValues = tree.filterValues ? resolve(itemData, tree.filterValues) : [];
	  const values = Array.isArray(filterValues) ? filterValues.map(String) : [];
	  if (filter && filter !== 'all' && values.length && !values.includes(filter)) return null;
	  if (filter && filter !== 'all' && Array.isArray(rawChildren) && rawChildren.length && children.length === 0) return null;
	  const key = String(resolve(itemData, tree.nodeKey) ?? '').trim();
	  if (!key) throw new Error('Empty tree node key at ' + context.path);
	  return {raw, itemData, children, depth, key};
	}

	function preparedList(values, depth) {
	  const entries = (Array.isArray(values) ? values : []).map(item => prepared(item, depth)).filter(Boolean);
	  const seen = new Set();
	  entries.forEach(entry => {
		if (seen.has(entry.key)) throw new Error('Duplicate tree node key "' + entry.key + '" at ' + context.path);
		seen.add(entry.key);
	  });
	  return entries;
	}

    function renderEntry(entry, parentIdentity = context.identity) {
	  const itemData = entry.itemData;
	  const entryKey = entry.key;
	  const entryIdentity = parentIdentity + '/tree-node:' + rendererKeyPart(entryKey);
	  const row = document.createElement('div');
	  row.className = 'dsl-tree-row';
	  annotateRendererElement(row, 'tree-row', entryIdentity + '/row');
	  row.style.setProperty('--ciwi-tree-depth', String(entry.depth));
	  let link = tree.nodeLink ? String(resolve(itemData, tree.nodeLink) || '') : '';
	  const fileDownload = (node.actions || []).find(action => action.command === 'download-artifact');
	  if (!link && fileDownload) {
		const args = Object.fromEntries(Object.entries(fileDownload.arguments || {}).map(([key, value]) => [key, renderText({template: value}, itemData)]));
		if (String(args.kind || '') === 'file') {
		  const jobID = encodeURIComponent(args.jobExecutionId || '');
		  const artifactPath = String(args.path || '').split('/').map(encodeURIComponent).join('/');
		  link = '/artifacts/' + jobID + '/' + artifactPath;
		}
	  }
	  const label = renderText(tree.nodeLabel, itemData);
	  const labelElement = document.createElement(link ? 'a' : 'span');
	  labelElement.className = 'dsl-tree-label';
	  labelElement.textContent = label;
	  if (link) {
		labelElement.href = link;
		labelElement.target = '_blank';
		labelElement.rel = 'noopener noreferrer';
	  }
	  row.appendChild(labelElement);
	  const detail = tree.nodeDetail ? renderText(tree.nodeDetail, itemData) : '';
	  if (detail) {
		const detailElement = document.createElement('span');
		detailElement.className = 'dsl-tree-detail';
		detailElement.textContent = detail;
		const tone = tree.nodeTone ? semanticTone(resolve(itemData, tree.nodeTone)) : '';
		if (tone) detailElement.classList.add('dsl-' + tone);
		row.appendChild(detailElement);
	  }
	  const actionLabel = tree.actionLabel ? renderText(tree.actionLabel, itemData) : '';
	  if (actionLabel && Array.isArray(node.actions) && node.actions.length) {
		const button = document.createElement('button');
		button.className = 'dsl-button dsl-tree-action';
		button.type = 'button';
		button.textContent = actionLabel;
		const actionIdentity = entryIdentity + '/action';
		annotateRendererElement(button, 'button', actionIdentity);
		bindActions(button, node.actions, itemData, {session: context.session, identity: actionIdentity});
		row.appendChild(button);
	  }
	  if (!entry.children.length) return row;
	  const details = document.createElement('details');
	  details.className = 'dsl-tree-branch';
	  annotateRendererElement(details, 'tree-branch', entryIdentity);
	  const key = entryKey;
	  const stateKey = String(tree.stateKey || '') + ':' + key;
	  const fallback = tree.defaultExpanded ? !!resolve(itemData, tree.defaultExpanded) : false;
	  details.open = disclosureStates.get(stateKey, fallback);
	  details.dataset.disclosureKey = stateKey;
	  details.addEventListener('toggle', () => disclosureStates.set(stateKey, details.open));
	  const summary = document.createElement('summary');
	  summary.appendChild(row);
	  details.appendChild(summary);
	  const children = document.createElement('div');
	  children.className = 'dsl-tree-children';
	  entry.children.forEach(child => children.appendChild(renderEntry(child, entryIdentity)));
	  details.appendChild(children);
	  return details;
	}

	const preparedNodes = preparedList(source, 0);
	preparedNodes.forEach(entry => element.appendChild(renderEntry(entry)));
    }

    return {render: renderTreeView};
  };
})();
