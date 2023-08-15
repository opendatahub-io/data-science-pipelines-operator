import yaml


def table(rows, cols):
    """
    Convert a list of cits into a markdown table.

    Pre-condition: All dicts in list_of_dicts should have identical key_sets
    :param rows: list of dict where each key set for every dict matches list of cols
    :return: A markdown where each row corresponds to a dict in list_of_dicts
    """

    markdown_table = ""
    if len(rows) == 0:
        return markdown_table

    markdown_header = '| ' + ' | '.join(cols) + ' |'
    markdown_header_separator = '|-----' * len(cols) + '|'
    markdown_table += markdown_header + '\n'
    markdown_table += markdown_header_separator + '\n'
    for row in rows:
        markdown_row = ""
        for col in cols:
            markdown_row += '| ' + str(row[col]) + ' '
        markdown_row += '|' + '\n'
        markdown_table += markdown_row
    return markdown_table


def version_doc(args):
    input_file = args.input_file
    out_file = args.out_file
    with open(input_file, 'r') as f:
        versions = yaml.safe_load(f)

    with open('template/version_doc.md', 'r') as vd:
        final_md = vd.read()

    table_md = table(versions['rows'], versions['cols'])

    final_md = final_md.replace('<<GENERATED_COMPATIBILITY_TABLE>>', table_md)
    final_md = '<!--THIS DOC IS AUTO GENERATED-->\n' + final_md

    with open(out_file, 'w') as f:
        f.write(final_md)
