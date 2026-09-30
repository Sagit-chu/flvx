// Convert the WebAuthn JSON wire format to the browser's ArrayBuffer format.
function decode(value: string): ArrayBuffer {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/");
  const bytes = Uint8Array.from(atob(padded), (character) =>
    character.charCodeAt(0),
  );

  return bytes.buffer;
}

function encode(value: ArrayBuffer): string {
  const bytes = new Uint8Array(value);
  let binary = "";

  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte);
  });

  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

export async function createPasskey(options: {
  publicKey: Record<string, unknown>;
}) {
  const source = options.publicKey as unknown as {
    challenge: string;
    user: { id: string };
    excludeCredentials?: Array<{ id: string }>;
  };
  const publicKey: PublicKeyCredentialCreationOptions = {
    ...(options.publicKey as unknown as PublicKeyCredentialCreationOptions),
    challenge: decode(source.challenge),
    user: {
      ...(source.user as unknown as PublicKeyCredentialUserEntity),
      id: decode(source.user.id),
    },
    excludeCredentials: source.excludeCredentials?.map((item) => ({
      ...(item as unknown as PublicKeyCredentialDescriptor),
      id: decode(item.id),
    })),
  };
  const credential = (await navigator.credentials.create({
    publicKey,
  })) as PublicKeyCredential | null;

  if (!credential) throw new Error("未创建通行证密钥");
  const result = credential.response as AuthenticatorAttestationResponse;

  return {
    id: credential.id,
    rawId: encode(credential.rawId),
    type: credential.type,
    response: {
      attestationObject: encode(result.attestationObject),
      clientDataJSON: encode(result.clientDataJSON),
      transports: result.getTransports?.() || [],
    },
  };
}

export async function getPasskey(options: {
  publicKey: Record<string, unknown>;
}) {
  const source = options.publicKey as unknown as {
    challenge: string;
    allowCredentials?: Array<{ id: string }>;
  };
  const publicKey: PublicKeyCredentialRequestOptions = {
    ...(options.publicKey as unknown as PublicKeyCredentialRequestOptions),
    challenge: decode(source.challenge),
    allowCredentials: source.allowCredentials?.map((item) => ({
      ...(item as unknown as PublicKeyCredentialDescriptor),
      id: decode(item.id),
    })),
  };
  const credential = (await navigator.credentials.get({
    publicKey,
  })) as PublicKeyCredential | null;

  if (!credential) throw new Error("未选择通行证密钥");
  const result = credential.response as AuthenticatorAssertionResponse;

  return {
    id: credential.id,
    rawId: encode(credential.rawId),
    type: credential.type,
    response: {
      authenticatorData: encode(result.authenticatorData),
      clientDataJSON: encode(result.clientDataJSON),
      signature: encode(result.signature),
      userHandle: result.userHandle ? encode(result.userHandle) : null,
    },
  };
}
