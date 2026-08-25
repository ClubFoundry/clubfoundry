# Проверка выпуска контейнера-обновлятора

[English version](RELEASE_EVIDENCE.md)

Только тег `sidecar-v*` запускает публикацию образа обновлятора. Сборки веток и
pull request выполняют проверки, но ничего не публикуют.

Для разрешённого тега процесс выпуска публикует один индекс образов для
нескольких архитектур и сохраняет:

- неизменяемый digest OCI-манифеста;
- SPDX SBOM-attestation от BuildKit;
- BuildKit provenance максимального уровня;
- подписанную GitHub attestation, привязанную к неизменяемому digest;
- артефакт доказательств GitHub Actions с digest, экспортированными SBOM и
  provenance JSON, уведомлениями, текстами лицензий, предложением исходного
  кода и SHA-256 экспортированных файлов.

Процесс должен завершиться ошибкой, если инвентаризация пакетов отличается от
`runtime-packages.lock`, средство Docker не совпадает с
`runtime-tools.lock`, отсутствует или изменён обязательный текст лицензии, в
образе нет папки лицензий, обнаружена исправимая уязвимость Critical либо после
публикации невозможно выгрузить одну из требуемых attestation.

## Локальные проверки

```bash
python sidecar/verify-release-evidence.py
docker build --build-arg VERSION=evidence-check \
  -t clubfoundry-updater:evidence-check sidecar
bash sidecar/audit-runtime-licenses.sh clubfoundry-updater:evidence-check \
  | diff -u sidecar/runtime-packages.lock -
docker run --rm --entrypoint docker clubfoundry-updater:evidence-check \
  --version
docker run --rm --entrypoint docker clubfoundry-updater:evidence-check \
  compose version --short
```

## Проверка опубликованного образа

Замените пример digest значением из доказательств выпуска:

```bash
image=ghcr.io/clubfoundry/updater@sha256:EXACT_DIGEST
docker buildx imagetools inspect "$image" --format '{{json .SBOM}}'
docker buildx imagetools inspect "$image" --format '{{json .Provenance}}'
gh attestation verify "oci://$image" --repo ClubFoundry/clubfoundry
```

Тег образа, версия `/health` и маркер самообновления также должны сообщать одну
и ту же версию. Эта проверка идентичности выполняется при первом разрешённом
теге и не заменяется проверками цепочки поставки.
