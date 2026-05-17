from enum import Enum
import os
import re
import subprocess
import uuid
import yaml

class TriggerAction(Enum):
    email = 'email'
    webhook = 'webhook'

class TriggerCondition(Enum):
    success = 'success'
    failure = 'failure'
    always = 'always'

class Trigger:
    def __init__(self, yml):
        if not isinstance(yml, dict):
            raise Exception("Expected trigger to be a dict")
        self.action = TriggerAction(yml["action"])
        self.condition = TriggerCondition(yml["condition"])
        self.attrs = {
            key: yml[key] for key in yml.keys()
                if key not in ["action", "condition"]
        }

    def to_dict(self):
        attrs = self.attrs
        attrs.update({
            "action": self.action.value,
            "condition": self.condition.value,
        })
        return attrs

class Task:
    def __init__(self, yml):
        if not isinstance(yml, dict) or len(yml) != 1:
            raise Exception("Expected task to be a string: string")
        for key in yml:
            if not isinstance(key, str) or not isinstance(yml[key], str):
                raise Exception("Expected task to be a string: string")
            self.name = key
            self.script = yml[key].strip()
        if not re.match(r"^[a-z0-9_-]+$", self.name) or len(self.name) > 128:
            raise Exception(f"Task name '{self.name}' is invalid (must be all lowercase letters, " +
                "numbers, underscores, and dashes, and <=128 characters)")

    def __repr__(self):
        return "<Task {}>".format(self.name)

class Manifest:
    def __init__(self, yml):
        self.yaml = yml
        image = self.yaml.get("image")
        arch = self.yaml.get("arch")
        packages = self.yaml.get("packages")
        repos = self.yaml.get("repositories")
        sources = self.yaml.get("sources")
        env = self.yaml.get("environment")
        secrets = self.yaml.get("secrets")
        shell = self.yaml.get("shell")
        artifacts = self.yaml.get("artifacts")
        oauth = self.yaml.get("oauth")
        if not image:
            raise Exception("Missing image in manifest")
        if not isinstance(image, str):
            raise Exception("Expected image to be a string")
        if packages:
            if not isinstance(packages, list) or not all([isinstance(p, str) for p in packages]):
                raise Exception("Expected packages to be a string array")
        if repos:
            if not isinstance(repos, dict):
                raise Exception("Expected repositories to be a dict")
            for repo in repos:
                if not isinstance(repos[repo], str):
                    raise Exception("Expected url for repository {}".format(repo))
        if sources:
            if not isinstance(sources, list) or not all([isinstance(s, str) for s in sources]):
                raise Exception("Expected sources to be a string array")
        if env:
            if not isinstance(env, dict):
                raise Exception("Expected environment to be a dictionary")
        if secrets:
            if not isinstance(secrets, list) or not all(
                [isinstance(s, str) for s in secrets]
            ):
                raise Exception("Expected secrets to be a UUID/String array")

            def uuid_or_string(s):
                try:
                    return uuid.UUID(s)
                except ValueError:
                    if len(s) >= 3 and len(s) <= 512:
                        return s
                    raise Exception("Secret names must be between 3 and 512 chars")

            secrets = list(map(uuid_or_string, secrets))
        if shell is not None and not isinstance(shell, bool):
            raise Exception("Expected shell to be a boolean")
        if artifacts is not None and (
                not isinstance(artifacts, list) or
                any(not isinstance(p, str) or p == "" for p in artifacts) or
                len(set(os.path.basename(p) for p in artifacts)) != len(artifacts)):
            raise Exception("Expected artifacts to be a list of unique file paths")
        if oauth is not None and not isinstance(oauth, str):
            raise Exception("Expected oauth to be a string")
        self.image = image
        self.arch = arch
        self.packages = packages
        self.repos = repos
        self.sources = sources
        self.environment = env
        self.secrets = secrets
        self.shell = shell
        self.artifacts = artifacts
        self.oauth = oauth
        self.submitter = self.yaml.get("submitter")
        tasks = self.yaml.get("tasks")
        if not tasks or not isinstance(tasks, list):
            if (tasks is None or tasks == []) and not self.shell:
                raise Exception("Attempted to create manifest with no tasks")
            else:
                tasks = []
        self.tasks = [Task(t) for t in tasks]
        for task in self.tasks:
            if len([t for t in self.tasks if t.name == task.name]) != 1:
                raise Exception(f"Duplicate task '{task.name}'")
        triggers = self.yaml.get("triggers")
        self.triggers = [Trigger(t) for t in triggers] if triggers else []

    def __repr__(self):
        return "<Manifest {}, {} tasks>".format(self.image, len(self.tasks))
    
    def to_dict(self):
        return {
            "image": self.image,
            "arch": self.arch,
            "packages": self.packages,
            "repositories": self.repos,
            "sources": self.sources,
            "environment": self.environment,
            "secrets": [str(s) for s in self.secrets] if self.secrets else None,
            "submitter": self.submitter,
            "tasks": [{ t.name: t.script } for t in self.tasks],
            "triggers": [t.to_dict() for t in self.triggers] if any(self.triggers) else None,
            "shell": self.shell,
            "artifacts": self.artifacts,
            "oauth": self.oauth,
        }

    def to_yaml(self):
        return yaml.dump(self.to_dict())
