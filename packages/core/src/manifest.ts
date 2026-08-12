// The SHAPE of a frontend's self-introduction. The CONTENT belongs to each
// frontend — its token space and family id are its identity, not something
// four products share.

/** One themeable CSS custom property a build understands. */
export interface TokenSpec {
  name: string;
  kind: string;
  description?: string;
}

/** A validation rule a build needs and the deployment may not have. */
export interface KindSpec {
  name: string;
  pattern: string;
  description?: string;
}

export interface ThemeManifest {
  tokens: TokenSpec[];
  kinds?: KindSpec[];
  rules?: string;
}

export interface Manifest {
  familyId: string;
  buildHash: string;
  displayName: string;
  version: string;
  minApi: number;
  theme: ThemeManifest;
}
