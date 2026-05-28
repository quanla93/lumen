import type { en } from "./messages";

export type Locale = "en" | "vi";

type DotPrefix<TPrefix extends string, TKey extends string> = `${TPrefix}.${TKey}`;

type LeafKeys<T> = {
  [K in keyof T & string]: T[K] extends string
    ? K
    : T[K] extends Record<string, unknown>
      ? DotPrefix<K, LeafKeys<T[K]>>
      : never;
}[keyof T & string];

export type TranslationKey = LeafKeys<typeof en>;
export type TranslationParams = Record<string, string | number>;
