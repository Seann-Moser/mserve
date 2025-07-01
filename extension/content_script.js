(() => {
  let overlay, form, list, output, selectParentBtn, addChildBtn, selectorInput, nameInput, attrSelect, multipleCheckbox, childrenList, selectedParentPreview;
  let selecting = false, selectingChild = false;
  let hoverEl = null;
  let ruleset = [];
  let tempChildren = [];
  let selectedParent = null;

  // --- Helper Functions ---

  // Debounce utility function
  function debounce(func, delay) {
    let timeout;
    return function(...args) {
      const context = this;
      clearTimeout(timeout);
      timeout = setTimeout(() => func.apply(context, args), delay);
    };
  }

  /**
   * Escapes a string for use in a CSS selector (e.g., ID, class name, attribute value).
   * Handles special characters by escaping them.
   * @param {string} part The string to escape.
   * @returns {string} The escaped string.
   */
  function escapeCssSelectorPart(part) {
    if (window.CSS && window.CSS.escape) {
      return CSS.escape(part);
    }
    // Fallback for older browsers (less robust but covers common cases)
    return part.replace(/([!"#$%&'()*+,./:;<=>?@[\]^`{|}~])/g, '\\$1');
  }

  /**
   * Checks if a given CSS selector uniquely identifies the target element within the specified scope.
   * @param {string} selector The CSS selector to test.
   * @param {Element|Document} scope The DOM element or Document to query within.
   * @param {Element} targetEl The element that should be uniquely identified.
   * @returns {boolean} True if the selector uniquely identifies the target element within the scope, false otherwise.
   */
  function isUnique(selector, scope, targetEl) {
    if (!scope || !targetEl || !selector) {
      return false;
    }
    try {
      const elements = scope.querySelectorAll(selector);
      return elements.length === 1 && elements[0] === targetEl;
    } catch (e) {
      // console.warn(`Invalid selector '${selector}' for uniqueness check in scope ${scope.tagName || 'document'}:`, e);
      return false; // Invalid selector should not be considered unique
    }
  }

  /**
   * Generates a unique CSS selector for a given DOM element, relative to an optional context element.
   * Prioritizes IDs, data-test-id, unique classes, and falls back to nth-of-type.
   *
   * @param {Element} el The DOM element for which to generate a selector.
   * @param {Element} [contextEl=document.documentElement] The context element relative to which the selector should be unique.
   * @returns {string|null} The unique CSS selector, or null if one cannot be found (should be rare).
   */
  function getSelector(el, contextEl = document.documentElement) {
    if (!el || !(el instanceof Element)) {
      console.error("Invalid element provided to getSelector.");
      return null;
    }
    if (!contextEl || !(contextEl instanceof Element || contextEl === document)) {
      console.error("Invalid context element provided to getSelector.");
      return null;
    }
    if (!document.body.contains(el)) {
      console.warn("Element is not attached to the document. Cannot generate a selector.");
      return null;
    }

    // --- Initial check: If the element is the context itself ---
    if (el === contextEl) {
      // Try ID globally
      if (el.id) {
        const escapedId = escapeCssSelectorPart(el.id);
        const selector = `#${escapedId}`;
        if (isUnique(selector, document, el)) {
          return selector;
        }
      }

      // Try data-test-id globally
      const dataTestId = el.getAttribute('data-test-id');
      if (dataTestId) {
        const escapedDataId = escapeCssSelectorPart(dataTestId);
        const selector = `${el.tagName.toLowerCase()}[data-test-id="${escapedDataId}"]`;
        if (isUnique(selector, document, el)) {
          return selector;
        }
      }

      // Try unique class globally
      if (el.className) {
        const classes = el.className.split(' ').filter(cls => cls.trim() !== '');
        for (const cls of classes) {
          const potentialClassSelector = `.${escapeCssSelectorPart(cls)}`;
          const selector = `${el.tagName.toLowerCase()}${potentialClassSelector}`;
          if (isUnique(selector, document, el)) {
            return selector;
          }
        }
      }

      // If contextEl couldn't be uniquely identified by ID/data-test-id/class globally,
      // and it's not html/body, we'll try to get its full path relative to document.documentElement.
      // This is implicitly handled by the recursive call later if `parts` remains empty.
      if (el.tagName.toLowerCase() === 'html' || el.tagName.toLowerCase() === 'body') {
        return el.tagName.toLowerCase(); // 'html' or 'body' is always unique
      }
    }

    // --- Climb the DOM tree to build the selector path ---
    const parts = [];
    let currentEl = el;

    // Stop climbing when we reach contextEl or the html element
    while (currentEl && currentEl !== contextEl && currentEl.tagName.toLowerCase() !== 'html') {
      const tagName = currentEl.tagName.toLowerCase();
      let selectorPart = tagName; // Default to tag name if nothing more specific is found
      let foundStrongSelectorForThisLevel = false;

      // The scope for uniqueness checks for current element in the path:
      // If contextEl is an ancestor, use it as the scope. Otherwise, use parentElement.
      // Fallback to document if no parent (e.g., if currentEl is html/body)
      const queryScope = (contextEl.contains(currentEl) && contextEl !== document.documentElement) ? contextEl : (currentEl.parentElement || document);

      // 1. Try unique ID (highest priority, always check globally as IDs should be unique)
      if (currentEl.id) {
        const escapedId = escapeCssSelectorPart(currentEl.id);
        const potentialSelector = `#${escapedId}`;
        if (isUnique(potentialSelector, document, currentEl)) { // Check ID globally for robustness
          selectorPart = potentialSelector;
          foundStrongSelectorForThisLevel = true;
        }
      }

      // 2. Try unique data-test-id
      if (!foundStrongSelectorForThisLevel) {
        const dataTestId = currentEl.getAttribute('data-test-id');
        if (dataTestId) {
          const escapedDataId = escapeCssSelectorPart(dataTestId);
          const potentialAttrSelector = `${tagName}[data-test-id="${escapedDataId}"]`;
          if (isUnique(potentialAttrSelector, queryScope, currentEl)) {
            selectorPart = potentialAttrSelector;
            foundStrongSelectorForThisLevel = true;
          }
        }
      }

      // 3. Try other relevant attributes (name, role, itemprop)
      if (!foundStrongSelectorForThisLevel) {
        const commonAttrs = ['name', 'role', 'itemprop']; // Add more as needed
        for (const attrName of commonAttrs) {
          if (currentEl.hasAttribute(attrName)) {
            const attrValue = currentEl.getAttribute(attrName);
            if (attrValue) {
              const escapedAttrValue = escapeCssSelectorPart(attrValue);
              const potentialAttrSelector = `${tagName}[${attrName}="${escapedAttrValue}"]`;
              if (isUnique(potentialAttrSelector, queryScope, currentEl)) {
                selectorPart = potentialAttrSelector;
                foundStrongSelectorForThisLevel = true;
                break;
              }
            }
          }
        }
      }

      // 4. Try unique class name
      if (!foundStrongSelectorForThisLevel && currentEl.className) {
        const classes = currentEl.className.split(' ').filter(cls => cls.trim() !== '');
        for (const cls of classes) {
          const potentialClassSelector = `.${escapeCssSelectorPart(cls)}`;
          const selectorWithTag = `${tagName}${potentialClassSelector}`;
          if (isUnique(selectorWithTag, queryScope, currentEl)) {
            selectorPart = selectorWithTag;
            foundStrongSelectorForThisLevel = true;
            break; // Found a unique class for this element, no need to check others
          }
        }
      }

      // 5. Fallback to nth-of-type
      if (!foundStrongSelectorForThisLevel) {
        let nth = 1;
        let sibling = currentEl.previousElementSibling;
        while (sibling) {
          if (sibling.tagName.toLowerCase() === tagName) {
            nth++;
          }
          sibling = sibling.previousElementSibling;
        }

        // Add :nth-of-type only if there are multiple siblings of the same tag type within the parent.
        if (currentEl.parentElement) {
          const siblingsOfSameType = Array.from(currentEl.parentElement.children).filter(
              child => child.tagName.toLowerCase() === tagName
          );
          if (siblingsOfSameType.length > 1) {
            selectorPart += `:nth-of-type(${nth})`;
          }
        }
      }

      parts.unshift(selectorPart);

      // If we found a strong unique selector (like ID), we can sometimes stop climbing early.
      // This makes shorter, more robust selectors.
      if (foundStrongSelectorForThisLevel && selectorPart.startsWith('#')) {
        break; // Stop climbing if an ID was found
      }

      currentEl = currentEl.parentElement;
    }

    // --- Final Adjustments and Edge Cases ---

    // If we stopped climbing because we reached the context element,
    // prepend its selector if it's not html/body and not already handled.
    if (currentEl === contextEl && contextEl !== document.documentElement && contextEl !== document.body) {
      const contextSelector = getSelector(contextEl, document.documentElement); // Recursively get its global selector
      if (contextSelector && !parts.includes(contextSelector) && !(parts.length > 0 && parts[0] === contextSelector)) {
        parts.unshift(contextSelector);
      }
    }

    // If 'el' was the 'contextEl' and no unique selector was found in the initial check,
    // and no path was built (e.g., 'el' is a plain div with no unique attributes within 'document.documentElement').
    if (parts.length === 0 && el === contextEl) {
      return el.tagName.toLowerCase(); // Fallback to just the tag name for the context element
    }

    // If after all logic, parts is still empty (very unlikely with the current fallbacks), return null.
    if (parts.length === 0) {
      return null;
    }

    return parts.join(' > ');
  }

  // --- Overlay and UI Functions ---

  function createOverlay() {
    if (overlay) return;
    overlay = document.createElement('div');
    overlay.id = 'er-overlay';
    overlay.innerHTML = `
      <div id="er-panel">
        <button id="er-close">✕</button>
        <h3>Rule Builder</h3>
        <p id="er-status-message" style="font-style: italic; color: #555;"></p>
        <button id="er-select">Select Parent Element</button>
        <div id="er-selected-parent-preview" style="font-size:0.9em; margin-top:5px; display:none;">
            <strong>Parent:</strong> <code id="er-parent-selector-display"></code>
        </div>
        <form id="er-form" style="display:none;">
          <label>Selector<input id="er-selector" readonly></label>
          <div id="er-selector-preview"></div>
          <label>Name<input id="er-name"></label>
          <label>Attr<select id="er-attr"><option value="__text__">Text</option></select></label>
          <span id="er-attr-preview" style="font-size:0.8em; margin-left: 5px; color: #666;"></span>
          <label><input type="checkbox" id="er-multi"> Multiple</label>
          <div id="er-children-config">
            <h5>Children</h5>
            <button id="er-add-child-btn">Select Child Element</button>
            <ul id="er-children-list"></ul>
          </div>
          <button id="er-add">Add Rule</button>
        </form>
        <h4>Rules</h4><ul id="er-list"></ul>
        <button id="er-export">Copy JSON</button>
        <div id="er-copied-message" class="er-copied-message" style="display:none;">Copied!</div>
        <textarea id="er-output" readonly></textarea>
      </div>`;
    document.body.append(overlay);

    // Cache elements
    form = overlay.querySelector('#er-form');
    list = overlay.querySelector('#er-list');
    output = overlay.querySelector('#er-output');
    selectParentBtn = overlay.querySelector('#er-select');
    addChildBtn = overlay.querySelector('#er-add-child-btn');
    selectorInput = overlay.querySelector('#er-selector');
    nameInput = overlay.querySelector('#er-name');
    attrSelect = overlay.querySelector('#er-attr');
    multipleCheckbox = overlay.querySelector('#er-multi');
    childrenList = overlay.querySelector('#er-children-list');
    selectedParentPreview = overlay.querySelector('#er-selected-parent-preview');

    // Event Listeners
    overlay.querySelector('#er-close').onclick = removeOverlay;
    selectParentBtn.onclick = startSelecting;
    addChildBtn.onclick = startChildSelecting;
    overlay.querySelector('#er-add').onclick = addRule;
    overlay.querySelector('#er-export').onclick = () => {
      navigator.clipboard.writeText(JSON.stringify(ruleset, null,2));
      const copiedMsg = overlay.querySelector('#er-copied-message');
      copiedMsg.style.display = 'block';
      setTimeout(() => copiedMsg.style.display = 'none', 2000); // Hide after 2 seconds
    };

    attrSelect.onchange = updateAttrPreview; // New: Update attribute preview on change
    selectorInput.oninput = updateSelectorPreview; // New: Update selector preview if user manually edits (though it's readonly now)

    // Listen for Escape key to cancel selection
    document.addEventListener('keydown', handleKeydown);
    updateRuleListDisplay(); // Load any existing rules (if you implement persistence later)
  }

  function removeOverlay() {
    console.log("remove overlay")
    if (overlay) overlay.remove();
    teardownSelect();
    teardownChildSelect();
    overlay = null;
    ruleset = []; // Reset rules
    selectedParent = null; // Reset selected parent
    document.body.classList.remove('er-dimmed-body'); // Remove dimming
    document.removeEventListener('keydown', handleKeydown); // Clean up event listener
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      if (selecting) {
        teardownSelect();
      }
      if (selectingChild) {
        teardownChildSelect();
      }
      overlay.querySelector('#er-status-message').textContent = ''; // Clear status
    }
  }

  function startSelecting(e) {
    e.preventDefault();
    form.style.display = 'none';
    childrenList.innerHTML = ''; // Clear child list
    tempChildren = [];
    selectedParent = null;
    selectedParentPreview.style.display = 'none'; // Hide parent preview
    overlay.querySelector('#er-parent-selector-display').textContent = ''; // Clear parent selector display
    nameInput.value = ''; // Clear name input
    selectorInput.value = ''; // Clear selector input
    overlay.querySelector('#er-selector-preview').textContent = ''; // Clear selector preview
    attrSelect.innerHTML = '<option value="__text__">Text</option>'; // Reset attribute dropdown
    updateAttrPreview(); // Clear attr preview

    selectHandlers();
    selectParentBtn.textContent = 'Click to Select Parent... (ESC to cancel)'; // UX
    selectParentBtn.disabled = true; // UX
    addChildBtn.disabled = true; // UX
    document.body.classList.add('er-dimmed-body'); // Dim background
    overlay.querySelector('#er-status-message').textContent = 'Selecting Parent Element...';
  }

  function selectHandlers() {
    selecting = true;
    document.addEventListener('mouseover', debounce(highlight, 50)); // Debounce mouseover
    document.addEventListener('click', clickHandler, true);
  }

  function teardownSelect() {
    selecting = false;
    if (hoverEl) hoverEl.classList.remove('er-highlight');
    document.removeEventListener('mouseover', debounce(highlight, 50)); // Remove debounced listener
    document.removeEventListener('click', clickHandler, true);
    selectParentBtn.textContent = 'Select Parent Element'; // UX
    selectParentBtn.disabled = false; // UX
    addChildBtn.disabled = false; // UX
    document.body.classList.remove('er-dimmed-body'); // Remove dimming
    overlay.querySelector('#er-status-message').textContent = ''; // Clear status
    hoverEl = null; // Clear hovered element
  }

  function startChildSelecting(e) {
    e.preventDefault();
    teardownSelect(); // Ensure parent selection is off
    if (!selectedParent) {
      overlay.querySelector('#er-status-message').textContent = 'Please select a parent element first!'; // UX
      return;
    }
    selectingChild = true;
    document.addEventListener('mouseover', debounce(childHighlight, 50)); // Debounce mouseover
    document.addEventListener('click', childClickHandler, true);
    addChildBtn.textContent = 'Click to Select Child... (ESC to cancel)'; // UX
    selectParentBtn.disabled = true; // UX
    addChildBtn.disabled = true; // UX
    document.body.classList.add('er-dimmed-body'); // Dim background
    overlay.querySelector('#er-status-message').textContent = 'Selecting Child Element within Parent...';
  }

  function teardownChildSelect() {
    selectingChild = false;
    if (hoverEl) hoverEl.classList.remove('er-highlight');
    document.removeEventListener('mouseover', debounce(childHighlight, 50)); // Remove debounced listener
    document.removeEventListener('click', childClickHandler, true);
    addChildBtn.textContent = 'Select Child Element'; // UX
    selectParentBtn.disabled = false; // UX
    addChildBtn.disabled = false; // UX
    document.body.classList.remove('er-dimmed-body'); // Remove dimming
    overlay.querySelector('#er-status-message').textContent = ''; // Clear status
    hoverEl = null; // Clear hovered element
  }

  function highlight(e) {
    if (!selecting) return;
    if (hoverEl) hoverEl.classList.remove('er-highlight');
    hoverEl = e.target;
    hoverEl.classList.add('er-highlight');
    // Optional: show a live selector preview as a tooltip on hover
    // This requires more complex tooltip logic not directly in CSS/JS here,
    // but the `getSelector` function could be used.
  }

  function childHighlight(e) {
    if (!selectingChild) return;
    // Only highlight if the element is a child of the selected parent
    if (!selectedParent.contains(e.target) && e.target !== selectedParent) return;
    if (hoverEl) hoverEl.classList.remove('er-highlight');
    hoverEl = e.target;
    hoverEl.classList.add('er-highlight');
  }

  function clickHandler(e) {
    if (!selecting) return;
    e.preventDefault(); e.stopPropagation();
    teardownSelect();
    const el = e.target;
    selectedParent = el;
    const sel = getSelector(el); // Generate selector for the parent
    const attrs = Array.from(el.attributes).map(a => a.name);

    // Populate form fields
    selectorInput.value = sel;
    nameInput.value = el.tagName.toLowerCase(); // Pre-fill name
    overlay.querySelector('#er-parent-selector-display').textContent = sel; // Show parent selector
    selectedParentPreview.style.display = 'block'; // Show parent preview area

    attrSelect.innerHTML = '<option value="__text__">Text</option>' +
        attrs.map(a => `<option value="${a}">${a}</option>`).join('');
    updateAttrPreview(); // Update attribute preview for default selection
    updateSelectorPreview(); // Update selector preview for the main selector

    form.style.display = 'block'; // Show the form
    overlay.querySelector('#er-status-message').textContent = 'Parent element selected. Configure rule.';
  }

  function childClickHandler(e) {
    if (!selectingChild) return;
    if (!selectedParent.contains(e.target) && e.target !== selectedParent) { // Ensure it's within the parent
      // console.log("Clicked element is not within the selected parent.");
      // Perhaps give a visual cue or status message that it must be inside the parent
      return;
    }
    e.preventDefault(); e.stopPropagation();
    teardownChildSelect();

    const el = e.target;
    // Generate child selector relative to the selectedParent
    const sel = getSelector(el, selectedParent);
    const attrs = Array.from(el.attributes).map(a => a.name);

    // UX: Use prompt for attribute and name, but make it better.
    // For a better UX, consider a small, temporary modal for child configuration
    // instead of blocking prompts. For now, we'll stick to prompts but clarify.
    let chosenAttr = '';
    let name = el.tagName.toLowerCase();

    // Try to suggest a reasonable attribute or text content
    const possibleAttrs = ['alt', 'title', 'src', 'href', 'value', 'aria-label'];
    for(const attr of possibleAttrs) {
      if (el.hasAttribute(attr) && el.getAttribute(attr).trim() !== '') {
        chosenAttr = attr;
        break;
      }
    }
    if (!chosenAttr && el.textContent.trim() !== '') {
      chosenAttr = '__text__'; // Indicate text content
    }

    // A more user-friendly prompt or a mini-form within the panel for child attributes/name
    // For this example, we'll keep the prompt for simplicity, but know it's a UX bottleneck.
    const attrInput = prompt(
        `Choose attribute for child (leave blank or enter __text__ for text content).
Available attributes: ${attrs.join(', ')}`,
        chosenAttr === '__text__' ? '' : chosenAttr // Pre-fill
    );
    if (attrInput !== null) { // User didn't cancel
      chosenAttr = attrInput === '' ? '__text__' : attrInput;
    } else {
      // User cancelled, do not add child
      overlay.querySelector('#er-status-message').textContent = 'Child selection cancelled.';
      return;
    }


    const nameInputPrompt = prompt('Child field name:', el.tagName.toLowerCase());
    if (nameInputPrompt !== null) { // User didn't cancel
      name = nameInputPrompt || el.tagName.toLowerCase();
    } else {
      // User cancelled, do not add child
      overlay.querySelector('#er-status-message').textContent = 'Child selection cancelled.';
      return;
    }


    const previewText = (chosenAttr === '__text__')
        ? el.textContent.trim().substring(0, 50) + (el.textContent.trim().length > 50 ? '...' : '')
        : el.getAttribute(chosenAttr)?.substring(0, 50) + (el.getAttribute(chosenAttr)?.length > 50 ? '...' : '') || '[empty]';

    tempChildren.push({ name: name, selector: sel, attr: (chosenAttr === '__text__' ? '' : chosenAttr) });
    const li = document.createElement('li');
    li.innerHTML = `<strong>${name}</strong> (${(chosenAttr === '__text__' ? 'text' : chosenAttr)}): ${previewText} <button class="er-remove-child" data-index="${tempChildren.length - 1}">✕</button>`; // Add remove button
    childrenList.appendChild(li);

    li.querySelector('.er-remove-child').onclick = (event) => {
      const indexToRemove = parseInt(event.target.dataset.index);
      tempChildren.splice(indexToRemove, 1);
      childrenList.innerHTML = ''; // Clear and redraw list to update indices
      tempChildren.forEach((child, idx) => {
        const childLi = document.createElement('li');
        const childPreview = (child.attr === '')
            ? (selectedParent.querySelector(child.selector)?.textContent.trim().substring(0, 50) || '[N/A]')
            : (selectedParent.querySelector(child.selector)?.getAttribute(child.attr)?.substring(0, 50) || '[N/A]');
        childLi.innerHTML = `<strong>${child.name}</strong> (${(child.attr === '' ? 'text' : child.attr)}): ${childPreview} <button class="er-remove-child" data-index="${idx}">✕</button>`;
        childrenList.appendChild(childLi);
        childLi.querySelector('.er-remove-child').onclick = (e) => { // Re-attach listener
          const i = parseInt(e.target.dataset.index);
          tempChildren.splice(i, 1);
          childrenList.innerHTML = ''; // Re-render
          tempChildren.forEach((ch, c_idx) => { // Redraw
            const chl = document.createElement('li');
            const chp = (ch.attr === '') ? (selectedParent.querySelector(ch.selector)?.textContent.trim().substring(0, 50) || '[N/A]') : (selectedParent.querySelector(ch.selector)?.getAttribute(ch.attr)?.substring(0, 50) || '[N/A]');
            chl.innerHTML = `<strong>${ch.name}</strong> (${(ch.attr === '' ? 'text' : ch.attr)}): ${chp} <button class="er-remove-child" data-index="${c_idx}">✕</button>`;
            childrenList.appendChild(chl);
            chl.querySelector('.er-remove-child').onclick = (ev) => {
              const idxToRemove = parseInt(ev.target.dataset.index);
              tempChildren.splice(idxToRemove, 1);
              childrenList.innerHTML = '';
              tempChildren.forEach((c, i) => { // Recursive redraw, could be optimized
                const l = document.createElement('li');
                const p = (c.attr === '') ? (selectedParent.querySelector(c.selector)?.textContent.trim().substring(0, 50) || '[N/A]') : (selectedParent.querySelector(c.selector)?.getAttribute(c.attr)?.substring(0, 50) || '[N/A]');
                l.innerHTML = `<strong>${c.name}</strong> (${(c.attr === '' ? 'text' : c.attr)}): ${p} <button class="er-remove-child" data-index="${i}">✕</button>`;
                childrenList.appendChild(l);
                l.querySelector('.er-remove-child').onclick = (evt) => {
                  const remIdx = parseInt(evt.target.dataset.index);
                  tempChildren.splice(remIdx, 1);
                  // A more robust re-render that doesn't clear all:
                  updateChildrenListDisplay(); // Call a dedicated function
                };
              });
            };
          });
        };
      });
      updateChildrenListDisplay(); // Update list after removal
    };
    overlay.querySelector('#er-status-message').textContent = 'Child element added. Select more or Add Rule.';
  }

  function updateChildrenListDisplay() {
    childrenList.innerHTML = '';
    tempChildren.forEach((child, idx) => {
      const li = document.createElement('li');
      const childPreview = (child.attr === '')
          ? (selectedParent.querySelector(child.selector)?.textContent.trim().substring(0, 50) || '[N/A]')
          : (selectedParent.querySelector(child.selector)?.getAttribute(child.attr)?.substring(0, 50) || '[N/A]');
      li.innerHTML = `<strong>${child.name}</strong> (${(child.attr === '' ? 'text' : child.attr)}): ${childPreview} <button class="er-remove-child" data-index="${idx}">✕</button>`;
      childrenList.appendChild(li);
      li.querySelector('.er-remove-child').onclick = (e) => {
        const indexToRemove = parseInt(e.target.dataset.index);
        tempChildren.splice(indexToRemove, 1);
        updateChildrenListDisplay(); // Re-render to update indices correctly
      };
    });
  }

  function updateSelectorPreview() {
    const currentSelector = selectorInput.value;
    const previewEl = overlay.querySelector('#er-selector-preview');
    try {
      const matchedElements = document.querySelectorAll(currentSelector);
      if (matchedElements.length === 1) {
        previewEl.textContent = `Matches 1 element (unique)`;
        previewEl.style.color = 'green';
      } else if (matchedElements.length > 1) {
        previewEl.textContent = `Matches ${matchedElements.length} elements (not unique)`;
        previewEl.style.color = 'orange';
      } else {
        previewEl.textContent = `Matches 0 elements (check selector)`;
        previewEl.style.color = 'red';
      }
    } catch (e) {
      previewEl.textContent = `Invalid selector: ${e.message}`;
      previewEl.style.color = 'red';
    }
  }

  function updateAttrPreview() {
    const selectedAttr = attrSelect.value;
    const attrPreviewSpan = overlay.querySelector('#er-attr-preview');
    if (selectedParent) {
      if (selectedAttr === '__text__') {
        attrPreviewSpan.textContent = `Preview: "${selectedParent.textContent.trim().substring(0, 50)}..."`;
      } else {
        const attrValue = selectedParent.getAttribute(selectedAttr);
        attrPreviewSpan.textContent = `Preview: "${(attrValue || '[empty]').substring(0, 50)}..."`;
      }
    } else {
      attrPreviewSpan.textContent = '';
    }
  }

  function addRule(e) {
    e.preventDefault();

    // Basic validation
    if (!selectorInput.value) {
      overlay.querySelector('#er-status-message').textContent = 'Selector cannot be empty!';
      return;
    }
    if (!nameInput.value.trim()) {
      overlay.querySelector('#er-status-message').textContent = 'Rule name cannot be empty!';
      return;
    }

    const sel = selectorInput.value;
    const name = nameInput.value.trim();
    const attrVal = attrSelect.value;
    const attr = attrVal === '__text__' ? '' : attrVal;
    const mult = multipleCheckbox.checked;

    const rule = { name, selector: sel, attr, multiple: mult, download: false, flatten: false, children: tempChildren };
    ruleset.push(rule);
    updateRuleListDisplay(); // Update display and JSON output

    // Reset form after adding
    form.reset();
    childrenList.innerHTML = '';
    tempChildren = [];
    selectedParent = null; // Clear selected parent after rule is added
    selectedParentPreview.style.display = 'none'; // Hide parent preview
    overlay.querySelector('#er-parent-selector-display').textContent = ''; // Clear parent selector display
    form.style.display = 'none'; // Hide form
    updateAttrPreview(); // Clear attribute preview
    overlay.querySelector('#er-selector-preview').textContent = ''; // Clear selector preview
    overlay.querySelector('#er-status-message').textContent = 'Rule added successfully!';
  }

  function updateRuleListDisplay() {
    list.innerHTML = ''; // Clear current list
    ruleset.forEach((rule, index) => {
      const li = document.createElement('li');
      li.innerHTML = `
            <strong>${rule.name}</strong>
            <br>Selector: <code>${rule.selector}</code>
            <br>Attr: ${rule.attr || 'Text'}
            ${rule.multiple ? '<br>Multiple: Yes' : ''}
            ${rule.children.length > 0 ? `<br>Children: ${rule.children.length} fields` : ''}
            <button class="er-remove-rule" data-index="${index}" style="margin-left: 10px;">Remove</button>
        `;
      list.appendChild(li);

      // Attach event listener for the remove button
      li.querySelector('.er-remove-rule').onclick = (event) => {
        const indexToRemove = parseInt(event.target.dataset.index);
        ruleset.splice(indexToRemove, 1);
        updateRuleListDisplay(); // Re-render to update indices and display
        overlay.querySelector('#er-status-message').textContent = 'Rule removed.';
      };
    });
    output.value = JSON.stringify(ruleset, null, 2); // Update output JSON
  }

  // --- Message Listener ---
  browser.runtime.onMessage.addListener((m) => {
    if (m.action === 'show-overlay') createOverlay();
  });
})();