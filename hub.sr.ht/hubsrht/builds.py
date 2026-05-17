import email.utils
import json
import random
import re
import yaml
from flask import url_for
from fnmatch import fnmatch
from hubsrht.services.builds import BuildsClient, GraphQLClientGraphQLMultiError
from hubsrht.services.builds import TriggerCondition, Visibility
from hubsrht.services.builds import TriggerInput, EmailTriggerInput, TriggerType
from hubsrht.services.git import GitClient
from hubsrht.services.lists import ListsClient, ToolIcon
from hubsrht.types import SourceRepo, RepoType
from shlex import quote
from sqlalchemy import func
from srht.config import get_origin
from srht.crypto import fernet
from srht.graphql import InternalAuth
from yaml.error import YAMLError

_listssrht = get_origin("lists.sr.ht", external=True, default=None)

_patchset_url_re = re.compile(
    rf"""
    ^
    {re.escape(_listssrht)}
    /(?P<owner>~[a-z_][a-z0-9_-]+)
    /(?P<list>[\w.-]+)
    /patches
    /(?P<patchset_id>\d+)
    $
    """,
    re.VERBOSE,
) if _listssrht else None

def submit_patchset(ml, patchset):
    buildsrht = get_origin("builds.sr.ht", external=True, default=None)
    if not buildsrht:
        return None
    from buildsrht.manifest import Manifest, Task, Trigger

    project = ml.project
    auth = InternalAuth(project.owner)
    builds_client = BuildsClient(auth)
    git_client = GitClient(auth)
    lists_client = ListsClient(auth)

    patch_id = patchset.id
    patch_url = f"{ml.url()}/patches/{patch_id}"
    subject = patchset.subject
    prefix = patchset.prefix

    if not prefix:
        # TODO: More sophisticated matching is possible
        # - test if patch is applicable to a repo; see the following:
        #   https://github.com/libgit2/pygit2/pull/1019
        # Will be useful for mailing lists shared by many repositories
        return None

    repo = (SourceRepo.query
            .filter(SourceRepo.project_id == project.id)
            .filter(func.lower(SourceRepo.name) == prefix.lower())).one_or_none()
    if not repo:
        return None
    if repo.repo_type != RepoType.git:
        # TODO: support for hg.sr.ht
        return None

    repo = git_client.get_manifests(repo.owner.username, repo.name).user.repository
    assert repo is not None

    manifests = dict()
    if repo.multiple:
        for dirent in repo.multiple.object.entries.results:
            if not dirent.object:
                continue
            if not any(fnmatch(dirent.name, pat) for pat in ["*.yml", "*.yaml"]):
                continue
            manifests[dirent.name] = dirent.object.text
    elif repo.single_yml:
        manifests[".build.yml"] = repo.single_yml.object.text
    elif repo.single_yaml:
        manifests[".build.yaml"] = repo.single_yaml.object.text
    else:
        return None

    if len(manifests) > 4:
        keys = list(manifests.keys())
        random.shuffle(keys)
        manifests = { key: manifests[key] for key in keys[:4] }

    ids = []

    version = patchset.version
    if version == 1:
        version = ""
    else:
        version = f" v{version}"

    message_id = patchset.thread.root.message_id
    reply_to = patchset.thread.root.reply_to
    if reply_to:
        submitter = email.utils.parseaddr(reply_to)
    else:
        name = patchset.submitter.name
        address = patchset.submitter.address
        submitter = (name, address)

    build_note = f"""[{subject}][0]{version} from [{submitter[0]}][1]

[0]: {ml.url()}/patches/{patch_id}
[1]: mailto:{submitter[1]}"""

    for key, value in manifests.items():
        try:
            manifest = Manifest(yaml.safe_load(value))
        except YAMLError:
            lists_client.create_tool(
                    patchset_id=patch_id,
                    icon=ToolIcon.FAILED,
                    details=f"Failed to submit build: error parsing YAML")
            continue

        submit_build = True
        sub = manifest.submitter
        if sub != None and "hub.sr.ht" in sub:
            hub_sub = sub["hub.sr.ht"]
            if "enabled" in hub_sub:
                submit_build = hub_sub["enabled"]
        if not submit_build:
            continue

        tool_id = lists_client.create_tool(
                patchset_id=patch_id,
                icon=ToolIcon.PENDING,
                details=f"build pending: {key}").create_tool.id

        apply_script = _gen_apply_script(lists_client, ml, patchset)
        task = Task({
            "_apply_patch": apply_script,
        })
        manifest.tasks.insert(0, task)

        if not manifest.environment:
            manifest.environment = {}

        manifest.environment.setdefault("BUILD_SUBMITTER", "hub.sr.ht")
        manifest.environment.setdefault("BUILD_REASON", "patchset")
        manifest.environment.setdefault("PATCHSET_ID", patch_id)
        manifest.environment.setdefault("PATCHSET_URL", patch_url)

        # Add webhook trigger
        root = get_origin("hub.sr.ht", external=False)
        details = fernet.encrypt(json.dumps({
            "mailing_list": ml.id,
            "patchset_id": patch_id,
            "tool_id": tool_id,
            "name": key,
            "user": project.owner.canonical_name,
        }).encode()).decode()
        manifest.triggers.append(Trigger({
            "action": "webhook",
            "condition": "always",
            "url": root + url_for("webhooks.build_complete", details=details),
        }))

        try:
            manifest = yaml.dump(manifest.to_dict(), default_flow_style=False)
            job = builds_client.submit_build(
                    manifest=manifest,
                    note=build_note,
                    tags=[repo.name, "patches", key],
                    execute=False,
                    visibility=Visibility(repo.visibility.value)).submit
        except GraphQLClientGraphQLMultiError as err:
            details = ", ".join([e.message for e in err.errors])
            lists_client.update_tool(
                    tool_id=tool_id,
                    icon=ToolIcon.FAILED,
                    details=f"Failed to submit build: {details}")
            continue

        ids.append(job.id)
        build_url = f"{buildsrht}/{project.owner.canonical_name}/job/{job.id}"
        lists_client.update_tool(
                tool_id=tool_id,
                icon=ToolIcon.WAITING,
                details=f"[#{job.id}]({build_url}) running {key}")

    trigger = TriggerInput(
        type=TriggerType.EMAIL,
        condition=TriggerCondition.ALWAYS,
        email=EmailTriggerInput(
            to=email.utils.formataddr(submitter),
            cc=ml.posting_addr(),
            in_reply_to=f"<{message_id}>",
        )
    )
    builds_client.create_group(
        jobs=ids,
        triggers=[trigger],
        note=build_note)
    return ids

def _gen_apply_script(client, ml, patchset):
    # Note: one may be tempted to replace the temporary file by piping curl
    # into git directly. Do not be misled! It is necessary to have two separate
    # commands so that a patch which fails to apply fails the build. pipefail
    # is a bashism.
    # TODO: https://todo.sr.ht/~sircmpwn/builds.sr.ht/291
    script = f"""echo "Applying patch(es) from lists.sr.ht"
git config --global user.name 'builds.sr.ht'
git config --global user.email 'builds@sr.ht'
"""

    deps_seen = set()
    for email in patchset.patches.results:
        for trailer in email.patch.trailers:
            key, value = trailer.name, trailer.value
            if key != "Depends-on":
                continue
            patchset_url = value.strip()
            match = _patchset_url_re.match(patchset_url)
            if not match:
                continue
            if patchset_url in deps_seen:
                continue
            deps_seen.add(patchset_url)
            patch = client.get_patchset(match["patchset_id"]).patchset
            patchset_mbox = patchset_url + "/mbox"
            script += f"""echo "Applying" {quote(patch.subject)}
curl -sS {quote(patchset_mbox)} >/tmp/patch
git -C {quote(patch.prefix)} am -3 /tmp/patch
"""

    patch_mbox = f"{ml.url()}/patches/{patchset.id}/mbox"
    script += f"""curl -sS {quote(patch_mbox)} >/tmp/patch
git -C {quote(patchset.prefix)} am -3 /tmp/patch
"""

    return script
