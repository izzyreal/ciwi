(function () {
  'use strict';

  window.ciwiCreateBrowserSelectControl = function ({controls, appendPositionedIcon, icon}) {
    let activeBrowserSelect = null;

    function closeBrowserSelect() {
      if (!activeBrowserSelect) return;
      const {trigger, menu, onDocumentPointer, onWindowChange} = activeBrowserSelect;
      if (menu && menu.parentNode) menu.remove();
      if (trigger) trigger.setAttribute('aria-expanded', 'false');
      document.removeEventListener('pointerdown', onDocumentPointer, true);
      window.removeEventListener('resize', onWindowChange);
      window.removeEventListener('scroll', onWindowChange, true);
      activeBrowserSelect = null;
    }

    function layoutBrowserSelectMenu(trigger, menu) {
      const rect = trigger.getBoundingClientRect();
	const visuals = controls().select;
      const inset = visuals.viewportInset;
	const menuGap = visuals.menuGap;
      const availableWidth = Math.max(visuals.menuMinimumWidth, window.innerWidth - inset * 2);
      menu.style.maxWidth = availableWidth + 'px';
      menu.style.minWidth = Math.min(availableWidth, rect.width) + 'px';
      menu.style.left = Math.min(Math.max(inset, rect.left), Math.max(inset, window.innerWidth - inset - menu.offsetWidth)) + 'px';
      const below = window.innerHeight - rect.bottom - inset;
      const above = rect.top - inset;
      const placeAbove = menu.offsetHeight > below && above > below;
	const availableHeight = placeAbove ? above : below;
	menu.style.maxHeight = Math.max(visuals.menuMinimumHeight, Math.min(visuals.menuMaximumHeight, availableHeight)) + 'px';
      menu.style.top = (placeAbove ? Math.max(inset, rect.top - menu.offsetHeight - menuGap) : rect.bottom + menuGap) + 'px';
    }

    function selectedBrowserOption(trigger) {
	const state = trigger.__ciwiSelectState || {options: [], selectedValue: ''};
	return state.options.find(option => option.value === state.selectedValue)
	  || state.options[0]
	  || {value: state.selectedValue, label: state.selectedValue};
    }

    function updateBrowserSelectTrigger(trigger) {
	const state = trigger.__ciwiSelectState;
	if (!state) return;
	const selected = selectedBrowserOption(trigger);
	trigger.value = state.selectedValue;
	trigger.dataset.selectedLabel = selected.label;
	const label = trigger.querySelector(':scope > .dsl-select-label');
	if (label) label.textContent = selected.label;
	trigger.setAttribute('aria-expanded', String(!!activeBrowserSelect && activeBrowserSelect.trigger === trigger));
	requestAnimationFrame(() => {
	  if (!document.body.contains(trigger)) return;
	  const context = document.createElement('canvas').getContext('2d');
	  const computed = window.getComputedStyle(trigger);
	  context.font = computed.font;
	  const widest = state.options.reduce((width, option) => Math.max(width, context.measureText(option.label).width), 0);
	  const padding = parseFloat(computed.paddingLeft) + parseFloat(computed.paddingRight);
	  const visuals = controls().select;
	  const contentWidth = widest + padding + visuals.chevronSize + visuals.chevronGap;
	  trigger.style.width = 'min(100%, ' + String(Math.ceil(contentWidth)) + 'px)';
	});
    }

    function syncBrowserSelectMenu(active) {
	if (!active || !active.menu || !active.trigger.__ciwiSelectState) return;
	const {trigger, menu} = active;
	const state = trigger.__ciwiSelectState;
	const previousFocus = document.activeElement && document.activeElement.closest
	  ? document.activeElement.closest('.dsl-select-option')
	  : null;
	const focusedValue = previousFocus && menu.contains(previousFocus) ? String(previousFocus.dataset.value || '') : '';
	const existing = new Map(Array.from(menu.querySelectorAll(':scope > .dsl-select-option')).map(choice => [
	  String(choice.dataset.value || ''),
	  choice,
	]));
	state.options.forEach(option => {
	  let choice = existing.get(option.value);
	  if (!choice) {
		choice = document.createElement('button');
		choice.type = 'button';
		choice.className = 'dsl-select-option';
		choice.setAttribute('role', 'option');
		const check = document.createElement('span');
		check.className = 'dsl-select-check';
		const copy = document.createElement('span');
		choice.append(check, copy);
	  }
	  choice.dataset.value = option.value;
	  choice.setAttribute('aria-selected', String(option.value === state.selectedValue));
	  choice.querySelector('.dsl-select-check').textContent = option.value === state.selectedValue ? '✓' : '';
	  choice.lastElementChild.textContent = option.label;
	  menu.appendChild(choice);
	  existing.delete(option.value);
	});
	existing.forEach(choice => choice.remove());
	if (focusedValue) {
	  const retainedFocus = Array.from(menu.children).find(choice => choice.dataset.value === focusedValue);
	  if (retainedFocus && document.activeElement !== retainedFocus) retainedFocus.focus({preventScroll: true});
	}
    }

    function chooseBrowserSelectOption(trigger, value) {
	const state = trigger.__ciwiSelectState;
	if (!state) return;
	const option = state.options.find(candidate => candidate.value === value);
	if (!option) return;
	state.selectedValue = option.value;
	updateBrowserSelectTrigger(trigger);
	closeBrowserSelect();
	trigger.dispatchEvent(new Event('change', {bubbles: true}));
	trigger.focus();
    }

    function openBrowserSelect(trigger) {
	if (activeBrowserSelect && activeBrowserSelect.trigger === trigger) {
	  closeBrowserSelect();
	  return;
	}
	closeBrowserSelect();
	const menu = document.createElement('div');
	menu.className = 'dsl-select-menu';
	menu.setAttribute('role', 'listbox');
	menu.setAttribute('aria-label', trigger.getAttribute('aria-label') || 'Options');
	menu.addEventListener('click', event => {
	  const choice = event.target.closest('.dsl-select-option');
	  if (choice && menu.contains(choice)) chooseBrowserSelectOption(trigger, String(choice.dataset.value || ''));
	});
	menu.addEventListener('keydown', event => {
	  const choices = Array.from(menu.querySelectorAll(':scope > .dsl-select-option'));
	  if (!choices.length) return;
	  const selectedIndex = Math.max(0, choices.findIndex(choice => choice.dataset.value === trigger.value));
	  const currentIndex = choices.indexOf(document.activeElement);
	  const activeIndex = currentIndex >= 0 ? currentIndex : selectedIndex;
	  const focusAt = index => choices[(index + choices.length) % choices.length].focus();
	  if (event.key === 'ArrowDown') { event.preventDefault(); focusAt(activeIndex + 1); }
	  if (event.key === 'ArrowUp') { event.preventDefault(); focusAt(activeIndex - 1); }
	  if (event.key === 'Home') { event.preventDefault(); focusAt(0); }
	  if (event.key === 'End') { event.preventDefault(); focusAt(choices.length - 1); }
	  if (event.key === 'Escape') { event.preventDefault(); closeBrowserSelect(); trigger.focus(); }
	});
	document.body.appendChild(menu);
	const onDocumentPointer = event => {
	  if (!menu.contains(event.target) && event.target !== trigger && !trigger.contains(event.target)) closeBrowserSelect();
	};
	const onWindowChange = () => layoutBrowserSelectMenu(trigger, menu);
	activeBrowserSelect = {trigger, menu, onDocumentPointer, onWindowChange};
	trigger.setAttribute('aria-expanded', 'true');
	syncBrowserSelectMenu(activeBrowserSelect);
	layoutBrowserSelectMenu(trigger, menu);
	document.addEventListener('pointerdown', onDocumentPointer, true);
	window.addEventListener('resize', onWindowChange);
	window.addEventListener('scroll', onWindowChange, true);
	const choices = Array.from(menu.querySelectorAll(':scope > .dsl-select-option'));
	const selectedIndex = Math.max(0, choices.findIndex(choice => choice.dataset.value === trigger.value));
	choices[selectedIndex] && choices[selectedIndex].focus();
    }

    function configureBrowserSelect(trigger, options, selectedValue) {
      trigger.type = 'button';
      trigger.classList.add('dsl-select');
	trigger.__ciwiSelectState = {options: options.map(option => ({...option})), selectedValue};
      trigger.setAttribute('aria-haspopup', 'listbox');
      trigger.setAttribute('aria-expanded', 'false');
      const label = document.createElement('span');
      label.className = 'dsl-select-label';
	appendPositionedIcon(trigger, label, icon('chevron-down'), controls().select.chevronPosition);
	updateBrowserSelectTrigger(trigger);
      trigger.addEventListener('click', () => openBrowserSelect(trigger));
      trigger.addEventListener('keydown', event => {
        if (['Enter', ' ', 'ArrowDown', 'ArrowUp'].includes(event.key)) {
          event.preventDefault();
		openBrowserSelect(trigger);
        }
      });
    }

    function updateMounted(trigger) {
      updateBrowserSelectTrigger(trigger);
      if (!activeBrowserSelect || activeBrowserSelect.trigger !== trigger) return;
      syncBrowserSelectMenu(activeBrowserSelect);
      requestAnimationFrame(() => {
        if (activeBrowserSelect && activeBrowserSelect.trigger === trigger) {
          layoutBrowserSelectMenu(trigger, activeBrowserSelect.menu);
        }
      });
    }

    function disposeWithin(node) {
      if (!activeBrowserSelect || !node) return;
      if (node === activeBrowserSelect.trigger ||
          (node.nodeType === Node.ELEMENT_NODE && node.contains(activeBrowserSelect.trigger))) {
        closeBrowserSelect();
      }
    }

    return {configure: configureBrowserSelect, updateMounted, disposeWithin};
  };
})();
