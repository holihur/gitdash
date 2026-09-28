# Dart / Pub example

A minimal Dart package plus the commands to publish it to gitdash's `dart`
(Pub hosted) registry and consume it with the native `dart` client.

## Publish

```bash
# one-time: store the PAT as the token for this host (repo scope)
dart pub token add http://<host>/api/packages/dart/<owner>

# from the hello/ directory
dart pub publish --server http://<host>/api/packages/dart/<owner>
```

Manual upload (no Dart SDK required):

```bash
tar czf hello-0.1.0.tar.gz pubspec.yaml lib
curl -u <owner>:<PAT> -F file=@hello-0.1.0.tar.gz \
  http://<host>/api/packages/dart/<owner>/publish
```

## Consume

Reference the package in `pubspec.yaml`:

```yaml
dependencies:
  hello:
    hosted:
      name: hello
      url: http://<host>/api/packages/dart/<owner>
    version: ^0.1.0
```

```bash
dart pub get
```

The hosted API endpoints gitdash implements:

- `GET  /api/packages/dart/<owner>/api/packages/versions/new`
- `POST /api/packages/dart/<owner>/api/packages/versions/newUpload`
- `GET  /api/packages/dart/<owner>/api/packages/<name>`
- `GET  /api/packages/dart/<owner>/api/packages/<name>/versions/<version>`
- `GET  /api/packages/dart/<owner>/api/packages/<name>/download/<version>`
