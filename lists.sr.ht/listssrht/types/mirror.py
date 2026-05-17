import sqlalchemy as sa
from srht.database import Base

class Mirror(Base):
    __tablename__ = 'mirror'
    id = sa.Column(sa.Integer, primary_key=True)
    created = sa.Column(sa.DateTime, nullable=False)
    updated = sa.Column(sa.DateTime, nullable=False)
    configure_attempts = sa.Column(sa.Integer,
            nullable=False, server_default='0')
    configured = sa.Column(sa.Boolean, nullable=False, server_default='f')
    list = sa.orm.relationship("List", back_populates="mirror")

    mailer_sender = sa.Column(sa.Unicode)
    """
    The address of the mailing list's automated management interface. It's
    assumed that the first email sent to the mirror is the mailer.
    """

    list_subscribe = sa.Column(sa.Unicode)
    """The mailto: URL from List-Subscribe, or the user's initial input"""

    list_unsubscribe = sa.Column(sa.Unicode)
    """The mailto: URL from List-Unsubscribe"""

    list_post = sa.Column(sa.Unicode)
    """The mailto: URL from List-Post"""
