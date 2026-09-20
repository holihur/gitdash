---
title: "Docker / OCI images"
weight: 2
summary: "Push and pull private Docker images."
---

```bash
docker login <instance>
docker tag myimage <instance>/<owner>/myimage:latest
docker push <instance>/<owner>/myimage:latest
```

Use a PAT as the login password. Image blobs are access-controlled so private repositories are not leaked.
