// Just tell content script to show the overlay
document.getElementById('start').onclick = () => {
  browser.tabs.query({active: true, currentWindow: true})
      .then(tabs => browser.tabs.sendMessage(tabs[0].id, {action: 'show-overlay'}));
};