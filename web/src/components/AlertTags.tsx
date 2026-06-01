import { useCallback, useEffect, useMemo, useState } from "react";
import { Plus, Pencil, Trash2, Tag as TagIcon, Server } from "lucide-react";
import {
  ApiError,
  hostsApi,
  tagsApi,
  type Host,
  type Tag,
} from "@/lib/api";
import {
  ErrorText,
  Field,
  FieldInput,
  GhostButton,
  PrimaryButton,
} from "@/components/CenterCard";
import { EmptyState, IconButton, Surface } from "@/components/ui";
import { useConfirm } from "@/components/ConfirmDialog";
import { useI18n } from "@/i18n/useI18n";

// AlertTags is the Tags tab inside Alerts. Two panes:
//   left  — tag inventory CRUD (key + description + value list)
//   right — host ↔ tag assignment (each inventory key becomes one dropdown
//           per host; assignment writes through hostsApi.setTags).
// Both panes share the inventory state — editing left re-renders the
// dropdown options on the right.
export function AlertTags() {
  const { t } = useI18n();
  const [tags, setTags] = useState<Tag[]>([]);
  const [hosts, setHosts] = useState<Host[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [t, h] = await Promise.all([tagsApi.list(), hostsApi.list()]);
      setTags(t);
      setHosts(h);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return (
    <div className="space-y-4">
      {error && <ErrorText message={error} />}
      <div className="grid gap-4 lg:grid-cols-2">
        <TagInventory tags={tags} loading={loading} onChanged={refresh} t={t} />
        <HostAssignments
          tags={tags}
          hosts={hosts}
          loading={loading}
          onChanged={refresh}
          t={t}
        />
      </div>
    </div>
  );
}

// ─── Left pane: inventory CRUD ────────────────────────────────────────────────

function TagInventory({
  tags,
  loading,
  onChanged,
  t,
}: {
  tags: Tag[];
  loading: boolean;
  onChanged: () => Promise<void>;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [creating, setCreating] = useState(false);
  const [editingKey, setEditingKey] = useState<string | null>(null);

  return (
    <Surface>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight">
            <TagIcon size={16} strokeWidth={1.75} className="text-[color:var(--lumen-teal)]" />
            {t("alerts.tagsTab.title")}
          </h2>
          <p className="mt-1 text-sm text-[color:var(--color-muted)]">
            {t("alerts.tagsTab.description")}
          </p>
        </div>
        <GhostButton onClick={() => setCreating(true)} disabled={creating} className="inline-flex items-center gap-1">
          <Plus size={14} strokeWidth={2} />
          {t("alerts.tagsTab.newTag")}
        </GhostButton>
      </div>

      {creating && (
        <div className="mt-4">
          <TagForm
            onClose={() => setCreating(false)}
            onSaved={async () => {
              setCreating(false);
              await onChanged();
            }}
            t={t}
          />
        </div>
      )}

      <div className="mt-4">
        {loading ? (
          <p className="text-sm text-[color:var(--color-muted)]">{t("alerts.listLoading")}</p>
        ) : tags.length === 0 ? (
          <EmptyState
            title={t("alerts.tagsTab.empty")}
            detail={t("alerts.tagsTab.emptyHint")}
          />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
                <th className="px-2 py-2">{t("alerts.tagsTab.colKey")}</th>
                <th className="px-2 py-2">{t("alerts.tagsTab.colDescription")}</th>
                <th className="px-2 py-2">{t("alerts.tagsTab.colValues")}</th>
                <th className="px-2 py-2 text-right">{t("alerts.tagsTab.colHostCount")}</th>
                <th className="px-2 py-2 text-right">{t("alerts.tagsTab.colRuleCount")}</th>
                <th className="px-2 py-2 text-right">{t("alerts.tagsTab.colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {tags.map((tag) => (
                <TagRow
                  key={tag.key}
                  tag={tag}
                  expanded={editingKey === tag.key}
                  onToggle={() =>
                    setEditingKey((cur) => (cur === tag.key ? null : tag.key))
                  }
                  onChanged={onChanged}
                  t={t}
                />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </Surface>
  );
}

function TagRow({
  tag,
  expanded,
  onToggle,
  onChanged,
  t,
}: {
  tag: Tag;
  expanded: boolean;
  onToggle: () => void;
  onChanged: () => Promise<void>;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const confirm = useConfirm();

  async function deleteTag() {
    setError(null);
    let impact;
    try {
      impact = await tagsApi.impact(tag.key);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
      return;
    }
    const ok = await confirm({
      title: t("alerts.tagsTab.deleteTagTitle"),
      message: t("alerts.tagsTab.deleteTagConfirm", {
        key: tag.key,
        hosts: impact.host_count,
        rules: impact.rule_count,
      }),
      confirmLabel: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      const res = await tagsApi.remove(tag.key);
      window.alert(
        t("alerts.tagsTab.deletedSummary", {
          hosts: res.host_count,
          rules: res.rule_count,
        }),
      );
      await onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <tr className={`group border-t border-[color:var(--color-border)] align-top transition-colors ${expanded ? "bg-[color:var(--color-bg)]" : "hover:bg-[color:var(--color-bg)]/60"}`}>
        <td className="px-2 py-2.5 font-mono text-[color:var(--color-fg)]">{tag.key}</td>
        <td className="px-2 py-2.5 text-[color:var(--color-muted)]">
          {tag.description || "—"}
        </td>
        <td className="px-2 py-2.5">
          <div className="flex flex-wrap gap-1">
            {tag.values.length === 0 ? (
              <span className="text-xs text-[color:var(--color-muted)]">—</span>
            ) : (
              tag.values.map((v) => (
                <span
                  key={v}
                  className="rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2 py-0.5 text-xs lumen-num text-[color:var(--color-fg)]"
                >
                  {v === "" ? "(empty)" : v}
                </span>
              ))
            )}
          </div>
        </td>
        <td className="px-2 py-2.5 text-right lumen-num text-[color:var(--color-muted)]">
          {tag.host_count}
        </td>
        <td className="px-2 py-2.5 text-right lumen-num text-[color:var(--color-muted)]">
          {tag.rule_count}
        </td>
        <td className="px-2 py-2.5 text-right whitespace-nowrap">
          <div className="inline-flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
            <IconButton
              onClick={onToggle}
              disabled={busy}
              label={expanded ? t("alerts.cancel") : t("alerts.tagsTab.editTag")}
              className="h-8 w-8"
            >
              <Pencil size={14} strokeWidth={1.75} />
            </IconButton>
            <IconButton
              onClick={deleteTag}
              disabled={busy}
              label={t("alerts.delete")}
              className="h-8 w-8 hover:text-[color:var(--color-danger)]"
            >
              <Trash2 size={14} strokeWidth={1.75} />
            </IconButton>
          </div>
        </td>
      </tr>
      {error && (
        <tr>
          <td colSpan={6} className="px-2 pb-2">
            <ErrorText message={error} />
          </td>
        </tr>
      )}
      {expanded && (
        <tr>
          <td colSpan={6} className="px-2 pb-3">
            <TagEditPanel tag={tag} onChanged={onChanged} t={t} />
          </td>
        </tr>
      )}
    </>
  );
}

function TagEditPanel({
  tag,
  onChanged,
  t,
}: {
  tag: Tag;
  onChanged: () => Promise<void>;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [description, setDescription] = useState(tag.description);
  const [newValue, setNewValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const confirm = useConfirm();

  async function saveDesc() {
    setBusy(true);
    setError(null);
    try {
      await tagsApi.update(tag.key, description);
      await onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function addValue() {
    const v = newValue.trim();
    if (!v) return;
    setBusy(true);
    setError(null);
    try {
      await tagsApi.addValue(tag.key, v);
      setNewValue("");
      await onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  async function removeValue(v: string) {
    setError(null);
    let impact;
    try {
      impact = await tagsApi.valueImpact(tag.key, v);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
      return;
    }
    const ok = await confirm({
      title: t("alerts.tagsTab.deleteValueTitle"),
      message: t("alerts.tagsTab.deleteValueConfirm", {
        key: tag.key,
        value: v,
        hosts: impact.host_count,
        rules: impact.rule_count,
      }),
      confirmLabel: t("common.delete"),
      destructive: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      const res = await tagsApi.removeValue(tag.key, v);
      window.alert(
        t("alerts.tagsTab.deletedSummary", {
          hosts: res.host_count,
          rules: res.rule_count,
        }),
      );
      await onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-3">
      <Field label={t("alerts.tagsTab.fieldDescription")}>
        <div className="flex gap-2">
          <FieldInput
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder=""
          />
          <PrimaryButton onClick={saveDesc} disabled={busy}>
            {t("alerts.save")}
          </PrimaryButton>
        </div>
      </Field>

      <div>
        <span className="block text-xs uppercase tracking-wide text-[color:var(--color-muted)] mb-1">
          {t("alerts.tagsTab.colValues")}
        </span>
        <div className="flex flex-wrap gap-2">
          {tag.values.map((v) => (
            <span
              key={v}
              className="inline-flex items-center gap-1 rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2 py-0.5 text-xs"
            >
              {v === "" ? "(empty)" : v}
              <button
                type="button"
                onClick={() => removeValue(v)}
                disabled={busy}
                className="ml-1 text-[color:var(--color-muted)] hover:text-[color:var(--color-danger)]"
                aria-label={t("alerts.tagsTab.removeValue")}
              >
                ×
              </button>
            </span>
          ))}
        </div>
        <div className="mt-2 flex gap-2">
          <FieldInput
            value={newValue}
            onChange={(e) => setNewValue(e.target.value)}
            placeholder={t("alerts.tagsTab.addValuePlaceholder")}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                void addValue();
              }
            }}
          />
          <GhostButton onClick={addValue} disabled={busy}>
            {t("alerts.tagsTab.addValue")}
          </GhostButton>
        </div>
      </div>
      {error && <ErrorText message={error} />}
    </div>
  );
}

function TagForm({
  onClose,
  onSaved,
  t,
}: {
  onClose: () => void;
  onSaved: () => Promise<void>;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [key, setKey] = useState("");
  const [description, setDescription] = useState("");
  const [valuesText, setValuesText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      const values = valuesText
        .split(/\r?\n/)
        .map((s) => s.trim())
        .filter((s) => s.length > 0);
      await tagsApi.create(key.trim(), description, values);
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3 rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] p-3">
      <Field label={t("alerts.tagsTab.fieldKey")}>
        <FieldInput
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="tier"
          autoFocus
        />
        <span className="mt-1 block text-xs text-[color:var(--color-muted)]">
          {t("alerts.tagsTab.fieldKeyHint")}
        </span>
      </Field>
      <Field label={t("alerts.tagsTab.fieldDescription")}>
        <FieldInput
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </Field>
      <Field label={t("alerts.tagsTab.fieldInitialValues")}>
        <textarea
          value={valuesText}
          onChange={(e) => setValuesText(e.target.value)}
          rows={4}
          className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-bg)] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)]"
          placeholder={"critical\nimportant\nnormal"}
        />
        <span className="mt-1 block text-xs text-[color:var(--color-muted)]">
          {t("alerts.tagsTab.fieldInitialValuesHint")}
        </span>
      </Field>
      <div className="flex items-center gap-2">
        <PrimaryButton onClick={submit} disabled={busy || !key.trim()}>
          {busy ? t("common.saving") : t("alerts.save")}
        </PrimaryButton>
        <GhostButton onClick={onClose} disabled={busy}>
          {t("alerts.cancel")}
        </GhostButton>
        {error && <ErrorText message={error} />}
      </div>
    </div>
  );
}

// ─── Right pane: host ↔ tag assignments ──────────────────────────────────────

function HostAssignments({
  tags,
  hosts,
  loading,
  onChanged,
  t,
}: {
  tags: Tag[];
  hosts: Host[];
  loading: boolean;
  onChanged: () => Promise<void>;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [editingHostID, setEditingHostID] = useState<number | null>(null);
  return (
    <Surface>
      <div>
        <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight">
          <Server size={16} strokeWidth={1.75} className="text-[color:var(--lumen-teal)]" />
          {t("alerts.tagsTab.hostsPaneTitle")}
        </h2>
        <p className="mt-1 text-sm text-[color:var(--color-muted)]">
          {t("alerts.tagsTab.hostsPaneDescription")}
        </p>
      </div>
      <div className="mt-4">
        {loading ? (
          <p className="text-sm text-[color:var(--color-muted)]">{t("alerts.listLoading")}</p>
        ) : hosts.length === 0 ? (
          <EmptyState title={t("alerts.tagsTab.hostsPaneEmpty")} />
        ) : tags.length === 0 ? (
          <EmptyState title={t("alerts.tagsTab.hostsPaneNoInventory")} />
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wide text-[color:var(--color-muted)]">
                <th className="px-2 py-2">{t("alerts.tagsTab.hostsHeader")}</th>
                <th className="px-2 py-2">{t("alerts.tagsTab.hostTagsHeader")}</th>
                <th className="px-2 py-2 text-right">{t("alerts.tagsTab.colActions")}</th>
              </tr>
            </thead>
            <tbody>
              {hosts.map((host) => (
                <HostAssignmentRow
                  key={host.id}
                  host={host}
                  tags={tags}
                  editing={editingHostID === host.id}
                  onStartEdit={() => setEditingHostID(host.id)}
                  onCloseEdit={() => setEditingHostID(null)}
                  onSaved={async () => {
                    setEditingHostID(null);
                    await onChanged();
                  }}
                  t={t}
                />
              ))}
            </tbody>
          </table>
        )}
      </div>
    </Surface>
  );
}

function HostAssignmentRow({
  host,
  tags,
  editing,
  onStartEdit,
  onCloseEdit,
  onSaved,
  t,
}: {
  host: Host;
  tags: Tag[];
  editing: boolean;
  onStartEdit: () => void;
  onCloseEdit: () => void;
  onSaved: () => Promise<void>;
  t: ReturnType<typeof useI18n>["t"];
}) {
  const [draft, setDraft] = useState<Record<string, string>>(host.tags ?? {});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // When the host's saved tags change (parent refresh after another row's
  // save), pull the new state into the local draft so the row reflects it.
  const savedTags = useMemo(() => host.tags ?? {}, [host.tags]);
  useEffect(() => {
    if (!editing) setDraft(savedTags);
  }, [editing, savedTags]);

  const chips = useMemo(() => Object.entries(savedTags), [savedTags]);

  async function save() {
    setBusy(true);
    setError(null);
    try {
      // hostsApi.setTags replaces the whole set; drop keys whose value is
      // the sentinel empty-string-via-deletion (we treat "" picked by the
      // "— none —" option as "remove this key").
      const cleaned: Record<string, string> = {};
      for (const [k, v] of Object.entries(draft)) {
        if (v === "__NONE__") continue;
        cleaned[k] = v;
      }
      await hostsApi.setTags(host.id, cleaned);
      await onSaved();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  if (!editing) {
    return (
      <tr className="group border-t border-[color:var(--color-border)] transition-colors hover:bg-[color:var(--color-bg)]/60">
        <td className="px-2 py-2.5 font-mono align-top text-[color:var(--color-fg)]">{host.name}</td>
        <td className="px-2 py-2.5 align-top">
          <div className="flex flex-wrap gap-1">
            {chips.length === 0 ? (
              <span className="text-xs text-[color:var(--color-muted)]">
                {t("settings.tagsEmpty")}
              </span>
            ) : (
              chips.map(([k, v]) => (
                <span
                  key={k}
                  className="rounded-full border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2 py-0.5 text-xs lumen-num text-[color:var(--color-fg)]"
                >
                  {v ? `${k}=${v}` : k}
                </span>
              ))
            )}
          </div>
        </td>
        <td className="px-2 py-2.5 text-right align-top whitespace-nowrap">
          <IconButton
            onClick={onStartEdit}
            label={t("alerts.tagsTab.hostEdit")}
            className="h-8 w-8 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100"
          >
            <Pencil size={14} strokeWidth={1.75} />
          </IconButton>
        </td>
      </tr>
    );
  }

  return (
    <tr className="border-t border-[color:var(--color-border)] bg-[color:var(--color-bg)]">
      <td className="px-2 py-3 font-mono align-top">{host.name}</td>
      <td className="px-2 py-3 align-top" colSpan={2}>
        <div className="grid gap-2 sm:grid-cols-2">
          {tags.map((tag) => {
            const current = draft[tag.key] ?? "__NONE__";
            return (
              <label key={tag.key} className="text-xs">
                <span className="block text-[color:var(--color-muted)] mb-0.5">
                  {tag.key}
                </span>
                <select
                  value={current}
                  onChange={(e) =>
                    setDraft({ ...draft, [tag.key]: e.target.value })
                  }
                  className="w-full rounded-md border border-[color:var(--color-border)] bg-[color:var(--color-card)] px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-[color:var(--color-accent)]"
                >
                  <option value="__NONE__">{t("alerts.tagsTab.hostNone")}</option>
                  {tag.values.map((v) => (
                    <option key={v} value={v}>
                      {v === "" ? "(empty)" : v}
                    </option>
                  ))}
                </select>
              </label>
            );
          })}
        </div>
        <div className="mt-3 flex items-center gap-2">
          <PrimaryButton onClick={save} disabled={busy}>
            {busy ? t("common.saving") : t("alerts.save")}
          </PrimaryButton>
          <GhostButton
            onClick={() => {
              setDraft(savedTags);
              setError(null);
              onCloseEdit();
            }}
            disabled={busy}
          >
            {t("alerts.cancel")}
          </GhostButton>
          {error && <ErrorText message={error} />}
        </div>
      </td>
    </tr>
  );
}
