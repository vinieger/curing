> *This fork was built to allow the poc to be run as container.*

---

# Build both
``` sh
docker-compose build

# Start the server
docker-compose up -d server
docker-compose logs -f server

# Run the client (one-shot)
docker-compose run --rm client


docker-compose down
```


## ⚠︎ Todo: Deploy to Kubernetes

### Upload to private registry (GKE)

``` sh
# Set image path
PROJECT_ID=$(gcloud config get-value project)
REPO="<FILLME>"
SERVER_IMAGE="$REPO/curing-server:latest"
CLIENT_IMAGE="$REPO/curing-client:latest"

# Tag local images to registry-qualified names (?)
docker tag curing-server:latest  $REGISTRY/curing-server:latest
docker tag curing-client:latest  $REGISTRY/curing-client:latest

# Build and push
gcloud auth configure-docker <FILLME>
docker-compose build curing-server
docker-compose build curing-client
docker push $SERVER_IMAGE
docker push $CLIENT_IMAGE


# Check it's there
gcloud artifacts docker images list "<FILLME>
kubectl config use-context <FILLME>
```

### K8s

``` sh
#!/usr/bin/env bash

NS="curing-poc"
SERVER_IMAGE=${REPO:curing-server:latest}
CLIENT_IMAGE=${REPO:curing-client:latest}

kubectl get ns "$NS" >/dev/null 2>&1 || kubectl create namespace "$NS"

kubectl -n "$NS" apply -f - <<YAML
apiVersion: v1
kind: ConfigMap
metadata:
  name: curing-config
data:
  config.server.json: |
    {
      "server": { "host": "0.0.0.0", "port": 8888 },
      "connect_interval_sec": 900
    }
  config.client.json: |
    {
      "server": { "host": "curing-server", "port": 8888 },
      "connect_interval_sec": 900
    }
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: curing-server
  labels: { app: curing-server }
spec:
  replicas: 1
  selector:
    matchLabels: { app: curing-server }
  template:
    metadata:
      labels: { app: curing-server }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 10001
      containers:
      - name: server
        image: ${SERVER_IMAGE}
        imagePullPolicy: IfNotPresent
        ports:
        - name: gob
          containerPort: 8888
        - name: health
          containerPort: 9090
        volumeMounts:
        - name: config
          mountPath: /app/cmd/config.json
          subPath: config.server.json
          readOnly: true
        readinessProbe:
          httpGet:
            path: /health
            port: health
          initialDelaySeconds: 2
          periodSeconds: 5
          timeoutSeconds: 2
          failureThreshold: 6
        livenessProbe:
          httpGet:
            path: /health
            port: health
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 2
          failureThreshold: 6
      volumes:
      - name: config
        configMap:
          name: curing-config
          items:
            - key: config.server.json
              path: config.server.json
---
apiVersion: v1
kind: Service
metadata:
  name: curing-server
  labels: { app: curing-server }
spec:
  selector: { app: curing-server }
  ports:
  - name: gob
    port: 8888
    targetPort: 8888
  type: ClusterIP
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: curing-client
  labels: { app: curing-client }
spec:
  replicas: 1
  selector:
    matchLabels: { app: curing-client }
  template:
    metadata:
      labels: { app: curing-client }
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 10002
      # Wait for server:8888 to accept TCP before starting the client (no nc required)
      initContainers:
      - name: wait-for-server
        image: bash:5.2
        command: ["bash","-lc","until (:</dev/tcp/curing-server/8888) 2>/dev/null; do echo 'waiting for server:8888'; sleep 1; done"]
      containers:
      - name: client
        image: ${CLIENT_IMAGE}
        imagePullPolicy: IfNotPresent
        securityContext:
          allowPrivilegeEscalation: false
          seccompProfile:
            type: Unconfined   # io_uring may need this for poc :(
        volumeMounts:
        - name: config
          mountPath: /app/cmd/config.json
          subPath: config.client.json
          readOnly: true
        - name: machine-id
          mountPath: /var/lib/machine-id
      volumes:
      - name: config
        configMap:
          name: curing-config
          items:
            - key: config.client.json
              path: config.client.json
      - name: machine-id
        emptyDir: {}
YAML


kubectl -n "$NS" rollout status deploy/curing-server
echo "Deployed to namespace '$NS'. Pods:"
kubectl -n "$NS" get pods -o wide
echo "Server service:"
kubectl -n "$NS" get svc curing-server
```