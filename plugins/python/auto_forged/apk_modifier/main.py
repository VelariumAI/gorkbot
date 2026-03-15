import sys
import json
import os
import subprocess
import tempfile
import shutil

def run_command(cmd, timeout=300):
    """Execute a bash command and return output."""
    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=True, text=True, timeout=timeout
        )
        return result.returncode, result.stdout, result.stderr
    except subprocess.TimeoutExpired:
        return -1, "", "Command timed out"
    except Exception as e:
        return -1, "", str(e)

def decompile_apk(apk_path, output_dir, tool="apktool"):
    """Decompile an APK file."""
    if tool == "apktool":
        cmd = f"apktool d \"{apk_path}\" -o \"{output_dir}\" -f"
    else:
        cmd = f"jadx -d \"{output_dir}\" \"{apk_path}\""
    
    code, stdout, stderr = run_command(cmd)
    if code == 0:
        return True, f"Decompiled to {output_dir}"
    return False, stderr

def rebuild_apk(source_dir, output_apk, keystore=None, alias=None, password=None):
    """Rebuild an APK from decompiled source."""
    # Build with apktool
    cmd = f"apktool b \"{source_dir}\" -o \"{output_apk}\""
    code, stdout, stderr = run_command(cmd)
    if code != 0:
        return False, f"Rebuild failed: {stderr}"
    
    # Align if zipalign available
    aligned_apk = output_apk.replace('.apk', '_aligned.apk')
    align_cmd = f"zipalign -f 4 \"{output_apk}\" \"{aligned_apk}\""
    code, _, _ = run_command(align_cmd)
    if code == 0:
        shutil.move(aligned_apk, output_apk)
    
    # Sign if keystore provided
    if keystore and os.path.exists(keystore):
        pass_arg = f"--pass {password}" if password else "--pass-inenv PIN_ENTRY"
        sign_cmd = f"apksigner sign --ks \"{keystore}\" --ks-key-alias \"{alias}\" {pass_arg} \"{output_apk}\""
        code, stdout, stderr = run_command(sign_cmd)
        if code != 0:
            return False, f"Sign failed: {stderr}"
        return True, f"Rebuilt, aligned, and signed: {output_apk}"
    
    return True, f"Rebuilt (unsigned): {output_apk}"

def install_apk(apk_path, package_name=None):
    """Install an APK via pm."""
    # Uninstall original first if package_name provided
    if package_name:
        uninstall_cmd = f"pm uninstall {package_name}"
        run_command(uninstall_cmd)
    
    install_cmd = f"pm install \"{apk_path}\""
    code, stdout, stderr = run_command(install_cmd)
    
    if code == 0:
        return True, f"Installed: {apk_path}"
    return False, stderr

def list_apk_contents(apk_path):
    """List contents of an APK file."""
    cmd = f"unzip -l \"{apk_path}\""
    code, stdout, stderr = run_command(cmd, timeout=60)
    
    if code == 0:
        return True, stdout
    return False, stderr

def extract_file_from_apk(apk_path, file_path, output_path):
    """Extract a specific file from an APK."""
    cmd = f"unzip -p \"{apk_path}\" \"{file_path}\" > \"{output_path}\""
    code, stdout, stderr = run_command(cmd)
    
    if code == 0:
        return True, f"Extracted {file_path} to {output_path}"
    return False, stderr

def main():
    try:
        input_data = json.loads(sys.stdin.read())
    except json.JSONDecodeError:
        print(json.dumps({"success": False, "output": "", "error": "Invalid JSON input"}))
        return
    
    action = input_data.get("action", "")
    apk_path = input_data.get("apk_path", "")
    output_path = input_data.get("output_path", "")
    
    # Optional parameters
    source_dir = input_data.get("source_dir", "")
    keystore = input_data.get("keystore", "")
    alias = input_data.get("alias", "")
    password = input_data.get("password", "")
    package_name = input_data.get("package_name", "")
    file_path = input_data.get("file_path", "")
    tool = input_data.get("tool", "apktool")
    
    success = False
    output = ""
    error = ""
    
    if action == "decompile":
        if not apk_path or not output_path:
            error = "Missing apk_path or output_path"
        else:
            success, output = decompile_apk(apk_path, output_path, tool)
            if not success:
                error = output
                output = ""
    
    elif action == "rebuild":
        if not source_dir or not output_path:
            error = "Missing source_dir or output_path"
        else:
            success, output = rebuild_apk(source_dir, output_path, keystore, alias, password)
            if not success:
                error = output
                output = ""
    
    elif action == "install":
        if not apk_path:
            error = "Missing apk_path"
        else:
            success, output = install_apk(apk_path, package_name)
            if not success:
                error = output
                output = ""
    
    elif action == "list":
        if not apk_path:
            error = "Missing apk_path"
        else:
            success, output = list_apk_contents(apk_path)
            if not success:
                error = output
                output = ""
    
    elif action == "extract_file":
        if not apk_path or not file_path or not output_path:
            error = "Missing apk_path, file_path, or output_path"
        else:
            success, output = extract_file_from_apk(apk_path, file_path, output_path)
            if not success:
                error = output
                output = ""
    
    else:
        error = f"Unknown action: {action}. Valid actions: decompile, rebuild, install, list, extract_file"
    
    result = {
        "success": success,
        "output": output,
        "error": error
    }
    
    print(json.dumps(result))

if __name__ == "__main__":
    main()
