import ws from 'k6/ws';
import { check } from 'k6';
import crypto from "k6/crypto";
import encoding from "k6/encoding";

const hmacSecret = "secret";

const algToHash = {
    HS256: "sha256",
    HS384: "sha384",
    HS512: "sha512"
};

function sign(data, hashAlg, secret) {
    let hasher = crypto.createHMAC(hashAlg, secret);
    hasher.update(data);
    // Some manual base64 encoding as `Hasher.digest(encodingType)` doesn't support that encoding type yet.
    return hasher.digest("base64").replace(/\//g, "_").replace(/\+/g, "-").replace(/=/g, "");
}

function encode(payload, secret, algorithm) {
    algorithm = algorithm || "HS256";
    let header = encoding.b64encode(JSON.stringify({ typ: "JWT", alg: algorithm }), "rawurl");
    payload = encoding.b64encode(JSON.stringify(payload), "rawurl");
    let sig = sign(header + "." + payload, algToHash[algorithm], secret);
    return [header, payload, sig].join(".");
}

// Generate a token for each user separately using HMAC JWT
function generateToken(userId) {
    const payload = {
        sub: userId,
        exp: Math.floor(Date.now() / 1000) + 60, // Expires in 60 seconds
    };
    return encode(payload, hmacSecret, "HS256")
}

export let options = {
    stages: [
        { duration: "10s", target: 1000 },
        { duration: "50s", target: 1000 },
    ]
};

export default function () {
    const url = 'ws://localhost:8000/connection/websocket';

    const user = `user_${__VU}_${__ITER}`;
    const token = generateToken(user);

    const response = ws.connect(url, {}, function (socket) {
        socket.on('open', () => {
            const connectCommand = JSON.stringify({ id: 1, connect: { token: token } });
            const subscribeCommand = JSON.stringify({ id: 2, subscribe: { channel: 'test' } });
            socket.send(connectCommand + "\n" + subscribeCommand);
        });

        socket.on('message', (message) => {
            // Respond to server pings.
            const substrings = message.split("\n");
            if (substrings.includes("{}")) {
                socket.send('{}');
            }
        });

        socket.on('close', () => {});

        socket.on('error', (error) => {
            console.log('WebSocket error: ', error);
        });

        socket.setTimeout(() => {
            socket.close();
        }, 60000);
    });

    check(response, { "status is 101": (r) => r && r.status === 101 });
}
