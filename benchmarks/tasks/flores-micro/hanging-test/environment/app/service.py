import time


class Service:
    def __init__(self, boot_steps=3):
        self.remaining = boot_steps

    def step(self):
        if self.remaining > 0:
            self.remaining -= 1

    @property
    def ready(self):
        return self.remaining == 0


def wait_until_ready(service, poll=0.01):
    """Advance the service until it is ready and return how many polls it took."""
    polls = 0
    while not service.ready:
        polls += 1
        time.sleep(poll)
    return polls
