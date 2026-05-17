from srht.search import search_by
from buildsrht.types import Job, JobStatus

def apply_search(query, terms):
    def job_image(value):
        return Job.image.ilike(f"%{value}%")

    def job_status(value):
        status = getattr(JobStatus, value, None)
        if status is None:
            raise ValueError(f"Invalid status: '{value}'")

        return Job.status == status

    def job_tags(value):
        return Job.tags.ilike(f"%{value}%")

    return search_by(query, terms, [Job.note], {
        "image": job_image,
        "status": job_status,
        "tags": job_tags,
    })
