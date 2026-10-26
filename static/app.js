// Keep normal HTML forms working; let htmx display server-side validation too.
document.addEventListener('htmx:beforeSwap', function (event) {
  if (event.detail.xhr.status === 422) {
    event.detail.shouldSwap = true;
    event.detail.isError = false;
  }
});
document.addEventListener('htmx:afterSwap', function () {
  document.getElementById('network-error').hidden = true;
  const error = document.querySelector('[data-focus-error]');
  if (error) error.focus();
});
for (const name of ['htmx:sendError', 'htmx:responseError', 'htmx:timeout']) {
  document.addEventListener(name, function () {
    document.getElementById('network-error').hidden = false;
  });
}
