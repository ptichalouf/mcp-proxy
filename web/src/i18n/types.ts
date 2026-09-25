import type { en } from './en';

export type DeepStringRecord<T> = {
  [K in keyof T]: T[K] extends Record<string, unknown>
    ? DeepStringRecord<T[K]>
    : string;
};

export type Dictionary = DeepStringRecord<typeof en>;

type Join<K, P> = K extends string | number
  ? P extends string | number
    ? `${K}.${P}`
    : never
  : never;

export type TranslationKey = {
  [K in keyof typeof en]: (typeof en)[K] extends Record<string, unknown>
    ? Join<K, keyof (typeof en)[K]>
    : K;
}[keyof typeof en];

export type InterpolationParams = Record<string, string | number>;
