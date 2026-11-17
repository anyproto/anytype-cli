# Portainer HOW-TO

Assuming you run portainer on one machine (not swarm), and assuming you self-host an automation platform like n8n.

1. Create a bridge network called `anytype_api`
2. Paste contents of docker-compose.yml in a new stack in Portainer
3. Depoy
4. Go to the stack and click the command line icon >_ next to the container.
5. Choose /bin/sh
6. In the shell, create a bot key using `./anytype auth create <yourbot>`.
7. Then make the bot join your space with `./anytype space join  <link>`
8. Then create an api key with `./anytype auth apikey create "n8n"`
9. Connect your n8n stack to the anytype cli stack (or put all in one stack)

    ```yaml
    services:
    n8n:
        # ....
        networks:
        - proxy
        - anytype_api
        
    networks:
    proxy:
        external: true
    anytype_api:
        external: true
    ```

10. Inside n8n test the connection by adding a CURL http request block.
    - URL: `http://anytype-cli:31012/v1/spaces`
    - Authentication: Generic
    - Auth type: Bearer
    - Bearer Auth: (create yours using the api key generated above)

...or paste this workflow in n8n.

```json
{
  "nodes": [
    {
      "parameters": {},
      "type": "n8n-nodes-base.manualTrigger",
      "typeVersion": 1,
      "position": [
        0,
        0
      ],
      "id": "d52e0dad-a11c-49ef-bec8-3fcc525f0669",
      "name": "When clicking ‘Execute workflow’"
    },
    {
      "parameters": {
        "url": "http://anytype-cli:31012/v1/spaces",
        "authentication": "genericCredentialType",
        "genericAuthType": "httpBearerAuth",
        "options": {}
      },
      "type": "n8n-nodes-base.httpRequest",
      "typeVersion": 4.4,
      "position": [
        208,
        0
      ],
      "id": "11fc3e3e-8de0-4da0-8eea-280d050c91af",
      "name": "HTTP Request",
      "credentials": {
        "httpBearerAuth": {
          "id": "Yenzl326lRAvHWQb",
          "name": "Anytype CLI"
        }
      }
    }
  ],
  "connections": {
    "When clicking ‘Execute workflow’": {
      "main": [
        [
          {
            "node": "HTTP Request",
            "type": "main",
            "index": 0
          }
        ]
      ]
    }
  },
  "pinData": {},
  "meta": {
    "instanceId": "ce110ceecbd52a55e2f86f58f176c40bfe61a2a2c6b384a681009bc6b9ef0dd4"
  }
}
```
