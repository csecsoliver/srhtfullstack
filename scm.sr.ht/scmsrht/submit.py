import os.path
import re
import yaml
from srht.config import cfg
from srht.graphql import exec_gql, GraphQLError
from urllib.parse import urlparse

builds_sr_ht = cfg("builds.sr.ht", "origin", None)

class BuildSubmitterBase:
    """ Base class for objects that can trigger a webhook on builds.sr.ht to
        start a job. There should be one sub-class of this per supported SCM.
    """
    def __init__(self, origin_url, scm_scheme, repo):
        self.origin_url = origin_url
        self.scm_scheme = scm_scheme
        self.repo = repo

    def submit(self, commit, user=None):
        result = SubmitResult()

        if not builds_sr_ht:
            return result.skip(["No build.sr.ht service"])

        # The `commit` parameter is some SCM-specific commit object we don't
        # necessarily know anything about.
        raw_manifests = self.find_manifests(commit)
        if not raw_manifests:
            return result.skip(["No build manifest found"])

        self._do_submit(raw_manifests, commit, result, user or self.repo.owner)

        return result

    def find_manifests(self, commit):
        # Should return a dictionary of manifest names and raw text contents.
        raise NotImplementedError()

    def get_commit_id(self, commit):
        # Get the id (hash) of the provided commit object.
        raise NotImplementedError()

    def get_commit_note(self, commit):
        # Builds a note to pass to the build service for this job.
        raise NotImplementedError()

    def get_clone_url(self, repo):
        # Returns a clone URL for this repo. If the repo is private, the clone
        # URL should probably be an SSH URL.
        raise NotImplementedError()

    def _do_submit(self, raw_manifests, commit, result, user):
        from buildsrht.manifest import Manifest
        manifests = {}
        commit_id = self.get_commit_id(commit)
        for name, txt in raw_manifests.items():
            try:
                m = yaml.safe_load(txt)
            except yaml.scanner.ScannerError as e:
                return result.fail([
                    f"Failed to submit build job {(' ' + name) if name else ''}:",
                    f"\t{str(e)}"
                    ])
            if not isinstance(m, dict):
                return result.fail([
                    f"Build manifest{(' ' + name) if name else ''} is invalid; expected a YAML dictionary."])
            self._auto_setup_manifest(m, result)
            self._add_commit_id_fragment(commit_id, m)
            manifests[name] = Manifest(m)

        commit_note = self.get_commit_note(commit)

        for name, manifest in iter(manifests.items()):
            params = {
                "manifest": yaml.dump(
                    manifest.to_dict(),
                    default_flow_style=False),
                "tags": [self.repo.name] + [name] if name else [],
                "note": commit_note,
            }

            try:
                r = exec_gql("builds.sr.ht", """
                    mutation Submit(
                        $manifest: String!,
                        $tags: [String!]!,
                        $note: String!,
                    ) {
                        submit(manifest: $manifest, tags: $tags, note: $note) {
                            id
                        }
                    }
                """, user=user, **params)
                job_id = r["submit"]["id"]
                result.add("Build started: {}/~{}/job/{} [{}]".format(
                    builds_sr_ht, self.repo.owner.username, job_id, name))
            except GraphQLError as err:
                result.add(f"Failed to submit build job {(' ' + name) if name else ''}:")
                result.add("\t" + ", ".join(e["message"] for e in err.errors))

        return result.success()

    def _auto_setup_sub_source(self, m, result):
        # If we find a source with the same basename as the current repo, we
        # make sure it's using the current repo's URL (it can be a different
        # URL if the curent repo is a fork).
        sources = m.get('sources', [])
        for i, src in enumerate(sources):
            if os.path.basename(src) == self.repo.name:
                clone_url = self.get_clone_url()
                sources[i] = clone_url

    def _auto_setup_auto_source(self, m, result):
        # If there's no source with the same basename as the current repo,
        # add the current repo URL in the sources.
        sources = m.get('sources')
        if sources is None:
            sources = []
            m['sources'] = sources
        for src in sources:
            srcurl = urlparse(src)
            srcurl_repo_name = os.path.basename(srcurl.path)
            if srcurl_repo_name == self.repo.name:
                break
        else:
            clone_url = self.get_clone_url()
            sources.append(clone_url)
            result.add("auto-setup: adding source {}".format(clone_url))

    def _auto_setup_manifest(self, m, result):
        self._auto_setup_sub_source(m, result)
        self._auto_setup_auto_source(m, result)

    def _add_commit_id_fragment(self, commit_id, m):
        sources = m.get('sources', [])
        for i, source in enumerate(sources):
            srcurl = urlparse(source)
            srcurl_repo_name = os.path.basename(srcurl.path)
            if srcurl_repo_name == self.repo.name:
                sources[i] = srcurl._replace(fragment=commit_id).geturl()

class SubmitResult:
    def __init__(self):
        self.msgs = []
        self.status = None

    def add(self, msg):
        self.msgs.append(msg)

    def success(self, msgs=None):
        return self._as('success', msgs)

    def fail(self, msgs=None):
        return self._as('failure', msgs)

    def skip(self, msgs=None):
        return self._as('skipped', msgs)

    def _as(self, status, msgs=None):
        if msgs is not None:
            self.msgs += msgs
        self.status = status
        return self

    def asdict(self):
        return {
            'status': self.status,
            'messages': self.msgs}

    def printmsgs(self):
        for msg in self.msgs:
            print(msg)
