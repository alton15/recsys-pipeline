# ranking-service.Dockerfile
# Exports the DCN-V2 model to ONNX format for Triton Inference Server.
#
# This is a build-time container that generates the ONNX model file.
# The actual serving is done by the Triton container using the model repository.

FROM python:3.12-slim AS builder

WORKDIR /app

# Install dependencies
RUN pip install --no-cache-dir \
    torch==2.5.1 --index-url https://download.pytorch.org/whl/cpu \
    onnx==1.17.0 \
    onnxruntime==1.20.1

# Copy export script
COPY services/ranking-service/scripts/export_model.py /app/export_model.py

# Export the model
RUN python export_model.py --output /models/dcn_v2/1/model.onnx

# --- Final stage: just the model artifact ---
FROM scratch AS model

COPY --from=builder /models /models
