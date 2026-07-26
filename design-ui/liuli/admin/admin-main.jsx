// Daycore Admin — entry
(function () {
  'use strict';
  const { useAdmin, ToastHost, Gate, Shell } = window.AdminShell;
  const A = window.AdminStore;
  function App() {
    useAdmin();
    return A.isAuthed() ? <Shell /> : <Gate />;
  }
  ReactDOM.createRoot(document.getElementById('root')).render(<ToastHost><App /></ToastHost>);
})();
