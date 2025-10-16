FROM python:3.11-slim

WORKDIR /app

# Add any additional dependencies here
# RUN pip install -r requirements.txt

COPY . /app

CMD ["python", "./script.py"]
