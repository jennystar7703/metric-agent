FROM python:3.9-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
# Use the stable Gunicorn server
CMD ["gunicorn", "--bind", "0.0.0.0:5001", "app:app"]