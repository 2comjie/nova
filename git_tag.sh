#!/usr/bin/env bash

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "用法: $0 v主版本.次版本.修订版本 [模块目录 ...]"
  echo "示例: $0 v0.1.5"
  echo "示例: $0 v0.1.3 locator/redis registry/redis"
  exit 1
fi

version="$1"
shift

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "版本格式错误: $version，应为 v0.1.5"
  exit 1
fi

repo_dir="$(cd "$(dirname "$0")" && pwd)"
cd "$repo_dir"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "工作区存在未提交修改，请先提交"
  exit 1
fi

if [[ $# -eq 0 ]]; then
  modules=(".")
else
  modules=("$@")
fi

tags=()
for module in "${modules[@]}"; do
  module="${module%/}"
  if [[ ! -f "$module/go.mod" ]]; then
    echo "模块不存在: $module"
    exit 1
  fi

  if [[ "$module" == "." ]]; then
    tag="$version"
  else
    tag="$module/$version"
  fi

  if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
    echo "Tag 已存在: $tag"
    exit 1
  fi
  tags+=("$tag")
done

for tag in "${tags[@]}"; do
  git tag -a "$tag" -m "release $tag"
done

git push origin "${tags[@]}"
printf '发布完成: %s\n' "${tags[*]}"
