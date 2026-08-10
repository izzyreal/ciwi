(function () {
  'use strict';

  window.ciwiCreateDOMReconciler = function ({selectControl}) {
    function rendererNodeKey(node) {
	return node && node.nodeType === Node.ELEMENT_NODE ? String(node.dataset.ciwiNodeKey || '') : '';
    }

    function compatibleRenderedNodes(current, next) {
	if (!current || !next || current.nodeType !== next.nodeType) return false;
	if (current.nodeType === Node.TEXT_NODE || current.nodeType === Node.COMMENT_NODE) return true;
	if (current.nodeType !== Node.ELEMENT_NODE) return false;
	if (current.namespaceURI !== next.namespaceURI || current.localName !== next.localName) return false;
	const currentKey = rendererNodeKey(current);
	const nextKey = rendererNodeKey(next);
	if ((currentKey || nextKey) && currentKey !== nextKey) return false;
	const currentComponent = String(current.dataset.ciwiComponent || '');
	const nextComponent = String(next.dataset.ciwiComponent || '');
	if ((currentComponent || nextComponent) && currentComponent !== nextComponent) return false;
	const currentIcon = String(current.dataset.ciwiIcon || '');
	const nextIcon = String(next.dataset.ciwiIcon || '');
	return !((currentIcon || nextIcon) && currentIcon !== nextIcon);
    }

    function patchRenderedAttributes(current, next) {
	Array.from(current.attributes).forEach(attribute => {
	  if (!next.hasAttribute(attribute.name)) current.removeAttribute(attribute.name);
	});
	Array.from(next.attributes).forEach(attribute => {
	  if (current.getAttribute(attribute.name) !== attribute.value) current.setAttribute(attribute.name, attribute.value);
	});
    }

    function patchRenderedElement(current, next) {
	const previousProgressState = String(current.__ciwiSemanticProgressState || '');
	const nextProgressState = String(next.__ciwiSemanticProgressState || '');
	const preserveProgressAnimation = previousProgressState !== '' && previousProgressState === nextProgressState;
	const previousProgressDelay = preserveProgressAnimation
	  ? current.style.getPropertyValue('--ciwi-progress-animation-delay')
	  : '';
	patchRenderedAttributes(current, next);
	if (preserveProgressAnimation) {
	  if (previousProgressDelay) current.style.setProperty('--ciwi-progress-animation-delay', previousProgressDelay);
	  else current.style.removeProperty('--ciwi-progress-animation-delay');
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiRenderedValue')) {
	  const previousValue = String(current.__ciwiRenderedValue ?? '');
	  const nextValue = String(next.__ciwiRenderedValue ?? '');
	  if (!Object.prototype.hasOwnProperty.call(current, '__ciwiRenderedValue') || previousValue !== nextValue) {
		current.value = nextValue;
	  }
	  current.__ciwiRenderedValue = nextValue;
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiRenderedDisabled')) {
	  current.__ciwiRenderedDisabled = !!next.__ciwiRenderedDisabled;
	  current.disabled = current.__ciwiRenderedDisabled;
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiSemanticProgress')) {
	  current.__ciwiSemanticProgress = next.__ciwiSemanticProgress;
	  current.__ciwiSemanticProgressState = preserveProgressAnimation ? previousProgressState : nextProgressState;
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiPulseTimestamp')) {
	  current.__ciwiPulseTimestamp = next.__ciwiPulseTimestamp;
	}
	if (Object.prototype.hasOwnProperty.call(next, '__ciwiSelectState')) {
	  current.__ciwiSelectState = {
		options: next.__ciwiSelectState.options.map(option => ({...option})),
		selectedValue: next.__ciwiSelectState.selectedValue,
	  };
	  selectControl().updateMounted(current);
	}
    }

    function disposeRenderedNode(node) {
	if (!node) return;
	selectControl().disposeWithin(node);
	if (node.nodeType !== Node.ELEMENT_NODE) return;
	const disposables = [node, ...node.querySelectorAll('*')].reverse();
	disposables.forEach(element => {
	  if (typeof element.__ciwiDispose !== 'function') return;
	  const dispose = element.__ciwiDispose;
	  element.__ciwiDispose = null;
	  dispose();
	});
    }

    function updateStatefulRenderedElement(current, next) {
	if (next.__ciwiStatefulContents === 'graph-view') {
	  const mountedViewports = new Map(Array.from(current.querySelectorAll('[data-ciwi-graph-viewport-key]')).map(viewport => [
		String(viewport.dataset.ciwiGraphViewportKey || ''),
		viewport,
	  ]));
	  const viewportKeys = node => {
		if (!node || node.nodeType !== Node.ELEMENT_NODE) return [];
		const result = node.matches('[data-ciwi-graph-viewport-key]') ? [node] : [];
		return result.concat(Array.from(node.querySelectorAll('[data-ciwi-graph-viewport-key]')))
		  .map(viewport => String(viewport.dataset.ciwiGraphViewportKey || ''));
	  };
	  const adoptViewport = nextViewport => {
		const mountedViewport = mountedViewports.get(String(nextViewport.dataset.ciwiGraphViewportKey || ''));
		if (!mountedViewport || typeof nextViewport.__ciwiAdoptGraphViewport !== 'function') return nextViewport;
		const mountedScaler = mountedViewport.querySelector(':scope > .dsl-definition-graph-scaler');
		const nextScaler = nextViewport.querySelector(':scope > .dsl-definition-graph-scaler');
		const mountedStage = mountedScaler && mountedScaler.querySelector(':scope > .dsl-definition-graph-stage');
		const nextStage = nextScaler && nextScaler.querySelector(':scope > .dsl-definition-graph-stage');
		if (!mountedScaler || !nextScaler || !mountedStage || !nextStage) return nextViewport;
		if (typeof mountedViewport.__ciwiDispose === 'function') mountedViewport.__ciwiDispose();
		patchRenderedAttributes(mountedScaler, nextScaler);
		patchRenderedAttributes(mountedStage, nextStage);
		Array.from(mountedStage.childNodes).forEach(disposeRenderedNode);
		mountedStage.replaceChildren(...Array.from(nextStage.childNodes));
		patchRenderedAttributes(mountedViewport, nextViewport);
		const adopt = nextViewport.__ciwiAdoptGraphViewport;
		nextViewport.__ciwiDispose = null;
		nextViewport.__ciwiAdoptGraphViewport = null;
		adopt(mountedViewport, mountedScaler, mountedStage);
		return mountedViewport;
	  };
	  const commitGraphSubtree = (mountedNode, nextNode) => {
		if (nextNode.matches('[data-ciwi-graph-viewport-key]')) return adoptViewport(nextNode);
		patchRenderedAttributes(mountedNode, nextNode);
		const previousChildren = Array.from(mountedNode.childNodes);
		const retained = new Set();
		const desiredChildren = Array.from(nextNode.childNodes).map(nextChild => {
		  const keys = viewportKeys(nextChild);
		  if (!keys.length) return nextChild;
		  const candidate = previousChildren.find(child => !retained.has(child) && viewportKeys(child).some(key => keys.includes(key)));
		  if (!candidate || candidate.nodeType !== Node.ELEMENT_NODE || nextChild.nodeType !== Node.ELEMENT_NODE) return nextChild;
		  retained.add(candidate);
		  return commitGraphSubtree(candidate, nextChild);
		});
		previousChildren.forEach(child => {
		  if (retained.has(child) || child.parentNode !== mountedNode) return;
		  disposeRenderedNode(child);
		  child.remove();
		});
		desiredChildren.forEach((child, index) => {
		  const reference = mountedNode.childNodes[index] || null;
		  if (child !== reference) mountedNode.insertBefore(child, reference);
		});
		return mountedNode;
	  };
	  commitGraphSubtree(current, next);
	} else {
	  Array.from(current.childNodes).forEach(disposeRenderedNode);
	  current.replaceChildren(...Array.from(next.childNodes));
	}
	current.__ciwiStatefulContents = next.__ciwiStatefulContents;
	current.__ciwiGraphRuntime = next.__ciwiGraphRuntime;
    }

    function reconcileRenderedChildren(current, next) {
	const previousChildren = Array.from(current.childNodes);
	const keyedChildren = new Map(previousChildren.map(child => [rendererNodeKey(child), child]).filter(([key]) => key));
	const nextKeys = new Set(Array.from(next.childNodes).map(rendererNodeKey).filter(Boolean));
	keyedChildren.forEach((child, key) => {
	  if (!nextKeys.has(key)) {
		disposeRenderedNode(child);
		child.remove();
	  }
	});
	const retained = new Set();
	Array.from(next.childNodes).forEach((nextChild, index) => {
	  const key = rendererNodeKey(nextChild);
	  let candidate = key ? keyedChildren.get(key) : current.childNodes[index];
	  if (candidate && retained.has(candidate)) candidate = null;
	  if (candidate && !key && rendererNodeKey(candidate)) candidate = null;
	  let committedChild = nextChild;
	  if (candidate && compatibleRenderedNodes(candidate, nextChild)) {
		committedChild = reconcileRenderedNode(candidate, nextChild);
		retained.add(candidate);
	  } else if (candidate && candidate.parentNode === current) {
		disposeRenderedNode(candidate);
		candidate.replaceWith(nextChild);
	  }
	  const reference = current.childNodes[index] || null;
	  if (committedChild !== reference) current.insertBefore(committedChild, reference);
	});
	previousChildren.forEach(child => {
	  if (!retained.has(child) && child.parentNode === current) {
		disposeRenderedNode(child);
		child.remove();
	  }
	});
    }

    function reconcileRenderedNode(current, next) {
	if (!compatibleRenderedNodes(current, next)) {
	  disposeRenderedNode(current);
	  current.replaceWith(next);
	  return next;
	}
	if (current.nodeType === Node.TEXT_NODE || current.nodeType === Node.COMMENT_NODE) {
	  if (current.data !== next.data) {
		const selection = current.nodeType === Node.TEXT_NODE && window.getSelection ? window.getSelection() : null;
		const selectionTouchesCurrent = selection && (selection.anchorNode === current || selection.focusNode === current);
		const anchorNode = selectionTouchesCurrent ? selection.anchorNode : null;
		const focusNode = selectionTouchesCurrent ? selection.focusNode : null;
		const anchorOffset = selectionTouchesCurrent ? selection.anchorOffset : 0;
		const focusOffset = selectionTouchesCurrent ? selection.focusOffset : 0;
		current.data = next.data;
		if (selectionTouchesCurrent && selection.setBaseAndExtent) {
		  const boundedOffset = (node, offset) => node === current ? Math.min(offset, current.data.length) : offset;
		  selection.setBaseAndExtent(
			anchorNode, boundedOffset(anchorNode, anchorOffset),
			focusNode, boundedOffset(focusNode, focusOffset),
		  );
		}
	  }
	  return current;
	}
	if (next.__ciwiStatefulContents) {
	  updateStatefulRenderedElement(current, next);
	  return current;
	}
	patchRenderedElement(current, next);
	reconcileRenderedChildren(current, next);
	if (typeof window.ciwiSyncActionElement === 'function') window.ciwiSyncActionElement(current);
	return current;
    }

    return {
      compatible: compatibleRenderedNodes,
      dispose: disposeRenderedNode,
      reconcile: reconcileRenderedNode,
    };
  };
})();
