<h2 align="center">
<img width="100" src="https://github.com/user-attachments/assets/abeb23ab-9857-483c-af22-a3f94dc08403">

A Discord web-based verification bot made in Go using [Fiber⚡️](https://github.com/gofiber/fiber)</h2>

# Setup
>[!NOTE]
> This uses a third-party service to check IP scores, this is optional and can be disabled by leaving "EMAIL" in the env file blank.\
> This also uses Google's reCaptcha V3 to verify the user without needing them to click any checkboxes.

- Create a Discord Bot
  - Enable the Server Members Intent
  - Give it these base permissions:
    - Manage Roles
    - Read/Send Messages
    - Embed Links
    - Use Application Commands
  - Copy the token and set the `"TOKEN"` in the .env file
- Fill in `.env.example` and rename it to `.env`

# TODO
- [X] Captcha implementation
- [X] Check for VPN/Proxy
- [ ] Prevent previously banned accounts from joining (something like ip checks or fingerprinting)
    * Might need to use FingerprintJS Pro for this, the open-src version is not accurate enough.
