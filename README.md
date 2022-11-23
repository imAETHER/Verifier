<h1 align="center">Verifier</h1>
<img src="https://aetherclient.com/uploads/imf44/7ea667ec59.png" width="100%">
<h3 align="center">A Discord web-based verification bot made in Go using Fiber</h3>

<hr>

This should be ready for release, it contains a simple verification screen and stuff, you only need to set it up.<br>
Do note that im still learning go.

# Bot Setup
- Create a Discord Bot
  - Enable the Server Members Intent
  - Give it these base permissions:
    - Manage Roles
    - Read/Send Messages
    - Embed Links
    - Use Application Commands
  - Copy the token and set the `"TOKEN"` in the .env file

# Usage
- Set the port in the .env file, make sure it has a leading `:`
- Set the `CAPTCHA_KEY` & `CAPTCHA_SECRET`, this uses Google's RECAPTCHAv3 (the hidden one)
- Set the `VERIFY_TIMEOUT`, this is how many minutes the verification link will be valid for (default is 2 minutes).

# TODO
- [X] Captcha implementation
- [X] Check for VPN/Proxy
- [ ] Prevent previously banned accounts from joining (something like ip checks or fingerprinting)
    * Might need to use FingerprintJS Pro for this, the open-src version is not accurate enough.
