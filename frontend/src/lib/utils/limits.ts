// Character caps the backend rejects over. Every free-text input carries the
// matching maxlength, so text the API would refuse never reaches the form.
// Keep in step with backend/internal/services/guest_service.go and the maxLen
// calls in backend/internal/handlers/.
export const GUEST_NAME_MAXLEN = 200;
export const GUEST_EMAIL_MAXLEN = 255;
export const GUEST_NOTES_MAXLEN = 2000;
export const GUEST_GIFT_MAXLEN = 200;

export const TABLE_NAME_MAXLEN = 100;

export const WEDDING_NAME_MAXLEN = 255;
export const VENUE_NAME_MAXLEN = 255;
export const VENUE_ADDRESS_MAXLEN = 500;
export const KIOSK_DESCRIPTION_MAXLEN = 2000;
export const ELEMENT_NAME_MAXLEN = 255;

export const ACCOUNT_NAME_MAXLEN = 255;
export const ACCOUNT_EMAIL_MAXLEN = 255;
// The backend counts bytes, so a multi-byte password can still be refused;
// maxlength only stops the obvious overrun.
export const PASSWORD_MAXLEN = 72;
