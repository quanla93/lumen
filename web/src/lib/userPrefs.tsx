import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  ApiError,
  DEFAULT_DASHBOARD_PREFS,
  DEFAULT_DISPLAY_PREFS,
  userPrefsApi,
  type DashboardPrefs,
  type DisplayPrefs,
} from "@/lib/api";

type PrefsState = {
  dashboard: DashboardPrefs;
  display: DisplayPrefs;
  ready: boolean;
  updateDashboard: (next: DashboardPrefs) => Promise<void>;
  updateDisplay: (next: DisplayPrefs) => Promise<void>;
};

const PrefsContext = createContext<PrefsState | null>(null);

// PrefsProvider loads /api/me/prefs once on mount. The first time the
// server returns null for display, we seed from any pre-existing
// localStorage (theme + locale) so users upgrading from <v0.6 don't
// have to re-pick their settings.
export function PrefsProvider({ children }: { children: ReactNode }) {
  const [dashboard, setDashboard] = useState<DashboardPrefs>(DEFAULT_DASHBOARD_PREFS);
  const [display, setDisplay] = useState<DisplayPrefs>(DEFAULT_DISPLAY_PREFS);
  const [ready, setReady] = useState(false);
  const seedAttempted = useRef(false);

  useEffect(() => {
    let cancelled = false;
    userPrefsApi.get()
      .then(async (resp) => {
        if (cancelled) return;
        const dash = resp.dashboard ?? DEFAULT_DASHBOARD_PREFS;
        let disp = resp.display ?? DEFAULT_DISPLAY_PREFS;

        if (!resp.display && !seedAttempted.current) {
          seedAttempted.current = true;
          const seeded = seedDisplayFromLocalStorage();
          if (seeded) {
            disp = seeded;
            userPrefsApi.putDisplay(seeded).catch(() => {});
          }
        }

        setDashboard(dash);
        setDisplay(disp);
        setReady(true);
      })
      .catch((err) => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 401) {
          // No session — caller hasn't mounted us under auth. Keep
          // defaults so UI still renders.
          setReady(true);
          return;
        }
        setReady(true);
      });
    return () => { cancelled = true; };
  }, []);

  const updateDashboard = useCallback(async (next: DashboardPrefs) => {
    const prev = dashboard;
    setDashboard(next);
    try {
      await userPrefsApi.putDashboard(next);
    } catch (err) {
      setDashboard(prev);
      throw err;
    }
  }, [dashboard]);

  const updateDisplay = useCallback(async (next: DisplayPrefs) => {
    const prev = display;
    setDisplay(next);
    try {
      await userPrefsApi.putDisplay(next);
    } catch (err) {
      setDisplay(prev);
      throw err;
    }
  }, [display]);

  const value = useMemo<PrefsState>(
    () => ({ dashboard, display, ready, updateDashboard, updateDisplay }),
    [dashboard, display, ready, updateDashboard, updateDisplay],
  );

  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}

export function usePrefs(): PrefsState {
  const ctx = useContext(PrefsContext);
  if (!ctx) {
    throw new Error("usePrefs must be used inside <PrefsProvider>");
  }
  return ctx;
}

// seedDisplayFromLocalStorage reads the legacy lumen.theme / lumen.locale
// keys once and returns DisplayPrefs if either is present. Returns null
// if neither key exists so PrefsProvider knows to keep the server-side
// null state and let the user pick fresh on first visit.
function seedDisplayFromLocalStorage(): DisplayPrefs | null {
  const theme = localStorage.getItem("lumen.theme");
  const locale = localStorage.getItem("lumen.locale");
  if (theme == null && locale == null) return null;
  const next: DisplayPrefs = { ...DEFAULT_DISPLAY_PREFS };
  if (theme === "dark" || theme === "light") next.theme = theme;
  if (locale === "en" || locale === "vi") next.language = locale;
  return next;
}

// applyDisplayPrefs runs once on every display change to push the
// values into the DOM (theme class on <html>, prefers-reduced-motion
// override). Body class for density is wired so PR3+ can hook in
// without further plumbing.
export function applyDisplayPrefs(d: DisplayPrefs) {
  const html = document.documentElement;
  const wantDark =
    d.theme === "dark" ||
    (d.theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  html.classList.toggle("dark", wantDark);

  // Reduce-motion: 'on'/'off' override the OS, 'system' clears the
  // override so the OS media-query wins.
  if (d.reduceMotion === "on") {
    html.dataset.reduceMotion = "on";
  } else if (d.reduceMotion === "off") {
    html.dataset.reduceMotion = "off";
  } else {
    delete html.dataset.reduceMotion;
  }

  html.dataset.density = d.density;
}
