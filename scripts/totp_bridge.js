const crypto = require('crypto');

function generateTOTP(secret) {
  const time = Math.floor(Date.now() / 1000 / 30);
  const buffer = Buffer.from(base32ToHex(secret.replace(/\s/g, '')), 'hex');
  
  const timeBuf = Buffer.alloc(8);
  for (let i = 7; i >= 0; i--) {
    timeBuf[i] = time & 0xff;
    time = time >> 8;
  }
  
  const hmac = crypto.createHmac('sha1', buffer);
  hmac.update(timeBuf);
  const hash = hmac.digest();
  
  const offset = hash[hash.length - 1] & 0xf;
  let code = ((hash[offset] & 0x7f) << 24) |
             ((hash[offset + 1] & 0xff) << 16) |
             ((hash[offset + 2] & 0xff) << 8) |
             (hash[offset + 3] & 0xff);
  
  code = (code % 1000000).toString().padStart(6, '0');
  return code;
}

function base32ToHex(base32) {
  const base32chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
  let bits = '';
  let hex = '';
  
  for (let i = 0; i < base32.length; i++) {
    const val = base32chars.indexOf(base32.charAt(i).toUpperCase());
    bits += val.toString(2).padStart(5, '0');
  }
  
  for (let i = 0; i + 4 <= bits.length; i += 4) {
    const chunk = bits.substr(i, 4);
    hex += parseInt(chunk, 2).toString(16);
  }
  return hex;
}

const secret = process.argv[2];
if (!secret) {
  console.error("Secret is required");
  process.exit(1);
}

try {
  console.log(generateTOTP(secret));
} catch (e) {
  console.error(`Error: ${e.message}`);
  process.exit(1);
}
