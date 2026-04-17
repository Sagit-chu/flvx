import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { registerSW } from "virtual:pwa-register";
import toast from "react-hot-toast";

import App from "./App.tsx";
import { Provider } from "./provider.tsx";
import "@/styles/globals.css";

const updateSW = registerSW({
  immediate: true,
  onNeedRefresh() {
    toast(
      (t) => (
        <div className="flex flex-col gap-3">
          <span className="text-sm font-medium text-foreground">发现新版本，是否立即刷新以应用更新？</span>
          <div className="flex gap-2 justify-end">
            <button
              className="px-3 py-1.5 text-xs font-medium bg-primary text-primary-foreground hover:bg-primary/90 rounded-md transition-colors"
              onClick={() => {
                updateSW(true);
                toast.dismiss(t.id);
              }}
            >
              刷新
            </button>
            <button
              className="px-3 py-1.5 text-xs font-medium bg-default-200 text-default-700 hover:bg-default-300 rounded-md transition-colors"
              onClick={() => toast.dismiss(t.id)}
            >
              稍后
            </button>
          </div>
        </div>
      ),
      { duration: Infinity, position: "bottom-right" }
    );
  },
});

ReactDOM.createRoot(document.getElementById("root")!).render(
  <BrowserRouter>
    <Provider>
      <App />
    </Provider>
  </BrowserRouter>,
);
