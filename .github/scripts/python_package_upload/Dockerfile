FROM docker.io/python:3.9

# Set working directory
WORKDIR /app

# Copy the script into the container
COPY package_download.sh /app/package_download.sh

# Make sure the script is executable
RUN chmod +x /app/package_download.sh

# Store the files in a folder
VOLUME /app/packages

ENTRYPOINT ["/app/package_download.sh"]
